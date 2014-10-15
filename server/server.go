package server

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/enproxy"
	"github.com/getlantern/flashlight/nattest"
	"github.com/getlantern/flashlight/statreporter"
	"github.com/getlantern/flashlight/statserver"
	"github.com/getlantern/go-igdman/igdman"
	log "github.com/getlantern/golog"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/keyman"
	"github.com/getlantern/nattywad"
)

const (
	PortmapFailure = 50
)

var (
	dialTimeout     = 10 * time.Second
	httpIdleTimeout = 70 * time.Second

	// Points in time, mostly used for generating certificates
	TEN_YEARS_FROM_TODAY = time.Now().AddDate(10, 0, 0)

	// Default TLS configuration for servers
	DEFAULT_TLS_SERVER_CONFIG = &tls.Config{
		// The ECDHE cipher suites are preferred for performance and forward
		// secrecy.  See https://community.qualys.com/blogs/securitylabs/2013/06/25/ssl-labs-deploying-forward-secrecy.
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
			tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
			tls.TLS_RSA_WITH_RC4_128_SHA,
			tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
	}
)

type Server struct {
	// Addr: listen address in form of host:port
	Addr string

	// ReadTimeout: (optional) timeout for read ops
	ReadTimeout time.Duration

	// WriteTimeout: (optional) timeout for write ops
	WriteTimeout time.Duration

	CertContext                *CertContext           // context for certificate management
	AllowNonGlobalDestinations bool                   // if true, requests to LAN, Loopback, etc. will be allowed
	StatReporter               *statreporter.Reporter // optional reporter of stats
	StatServer                 *statserver.Server     // optional server of stats

	host           string
	nattywadServer *nattywad.Server
	cfg            *ServerConfig
	cfgMutex       sync.Mutex
}

func (server *Server) Configure(newCfg *ServerConfig) {
	server.cfgMutex.Lock()
	defer server.cfgMutex.Unlock()

	oldCfg := server.cfg

	log.Debug("Server.Configure() called")
	if oldCfg != nil && reflect.DeepEqual(oldCfg, newCfg) {
		log.Debugf("Server configuration unchanged")
		return
	}

	server.host = newCfg.AdvertisedHost

	if oldCfg == nil || newCfg.Portmap != oldCfg.Portmap {
		// Portmap changed
		if oldCfg != nil && oldCfg.Portmap > 0 {
			log.Debugf("Attempting to unmap old external port %d", oldCfg.Portmap)
			err := unmapPort(oldCfg.Portmap)
			if err != nil {
				log.Errorf("Unable to unmap old external port: %s", err)
			}
			log.Debugf("Unmapped old external port %d", oldCfg.Portmap)
		}

		if newCfg.Portmap > 0 {
			log.Debugf("Attempting to map new external port %d", newCfg.Portmap)
			err := mapPort(server.Addr, newCfg.Portmap)
			if err != nil {
				log.Errorf("Unable to map new external port: %s", err)
				os.Exit(PortmapFailure)
			}
			log.Debugf("Mapped new external port %d", newCfg.Portmap)
		}
	}

	if newCfg.WaddellAddr != "" || server.nattywadServer != nil {
		if server.nattywadServer == nil {
			server.nattywadServer = &nattywad.Server{
				OnFiveTuple: func(local *net.UDPAddr, remote *net.UDPAddr) bool {
					err := nattest.Serve(local)
					if err != nil {
						log.Error(err.Error())
						return false
					}
					return true
				},
			}
		}
		server.nattywadServer.Configure(newCfg.WaddellAddr)
	}

	server.cfg = newCfg
}

// CertContext encapsulates the certificates used by a Server
type CertContext struct {
	PKFile         string
	ServerCertFile string
	PK             *keyman.PrivateKey
	ServerCert     *keyman.Certificate
}

func (server *Server) ListenAndServe() error {
	err := server.CertContext.InitServerCert(strings.Split(server.Addr, ":")[0])
	if err != nil {
		return fmt.Errorf("Unable to init server cert: %s", err)
	}

	// Set up an enproxy Proxy
	proxy := &enproxy.Proxy{
		Dial: server.dialDestination,
		Host: server.host,
	}

	if server.host != "" {
		log.Debugf("Running as host %s", server.host)
	}

	// Hook into stats reporting if necessary
	reportingStats := server.startReportingStatsIfNecessary()
	servingStats := server.startServingStatsIfNecessary()

	if reportingStats || servingStats {
		// Add callbacks to track bytes given
		proxy.OnBytesReceived = func(ip string, bytes int64) {
			if reportingStats {
				server.StatReporter.OnBytesGiven(ip, bytes)
			}
			if servingStats {
				server.StatServer.OnBytesReceived(ip, bytes)
			}
		}
		proxy.OnBytesSent = func(ip string, bytes int64) {
			if reportingStats {
				server.StatReporter.OnBytesGiven(ip, bytes)
			}
			if servingStats {
				server.StatServer.OnBytesSent(ip, bytes)
			}
		}
	}

	proxy.Start()

	httpServer := &http.Server{
		Handler:      proxy,
		ReadTimeout:  server.ReadTimeout,
		WriteTimeout: server.WriteTimeout,
	}

	log.Debugf("About to start server (https) proxy at %s", server.Addr)

	tlsConfig := DEFAULT_TLS_SERVER_CONFIG
	cert, err := tls.LoadX509KeyPair(server.CertContext.ServerCertFile, server.CertContext.PKFile)
	if err != nil {
		return fmt.Errorf("Unable to load certificate and key from %s and %s: %s", server.CertContext.ServerCertFile, server.CertContext.PKFile, err)
	}
	tlsConfig.Certificates = []tls.Certificate{cert}

	listener, err := tls.Listen("tcp", server.Addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("Unable to listen for tls connections at %s: %s", server.Addr, err)
	}

	// We use an idle timing listener to time out idle HTTP connections, since
	// the CDNs seem to like keeping lots of connections open indefinitely.
	idleTimingListener := idletiming.Listener(listener, httpIdleTimeout, nil)
	return httpServer.Serve(idleTimingListener)
}

// dialDestination dials the destination server and wraps the resulting net.Conn
// in a countingConn if an InstanceId was configured.
func (server *Server) dialDestination(addr string) (net.Conn, error) {
	if !server.AllowNonGlobalDestinations {
		host := strings.Split(addr, ":")[0]
		ipAddr, err := net.ResolveIPAddr("ip", host)
		if err != nil {
			err = fmt.Errorf("Unable to resolve destination IP addr: %s", err)
			log.Error(err.Error())
			return nil, err
		}
		if !ipAddr.IP.IsGlobalUnicast() {
			err = fmt.Errorf("Not accepting connections to non-global address: %s", host)
			log.Error(err.Error())
			return nil, err
		}
	}
	return net.DialTimeout("tcp", addr, dialTimeout)
}

// InitServerCert initializes a PK + cert for use by a server proxy, signed by
// the CA certificate.  We always generate a new certificate just in case.
func (ctx *CertContext) InitServerCert(host string) (err error) {
	if ctx.PK, err = keyman.LoadPKFromFile(ctx.PKFile); err != nil {
		if os.IsNotExist(err) {
			log.Debugf("Creating new PK at: %s", ctx.PKFile)
			if ctx.PK, err = keyman.GeneratePK(2048); err != nil {
				return
			}
			if err = ctx.PK.WriteToFile(ctx.PKFile); err != nil {
				return fmt.Errorf("Unable to save private key: %s", err)
			}
		} else {
			return fmt.Errorf("Unable to read private key, even though it exists: %s", err)
		}
	}

	log.Debugf("Creating new server cert at: %s", ctx.ServerCertFile)
	ctx.ServerCert, err = ctx.PK.TLSCertificateFor("Lantern", host, TEN_YEARS_FROM_TODAY, true, nil)
	if err != nil {
		return
	}
	err = ctx.ServerCert.WriteToFile(ctx.ServerCertFile)
	if err != nil {
		return
	}
	return nil
}

func mapPort(addr string, port int) error {
	parts := strings.Split(addr, ":")

	internalPort, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("Unable to parse local port: ")
	}

	internalIP := parts[0]
	if internalIP == "" {
		internalIP, err = determineInternalIP()
		if err != nil {
			return fmt.Errorf("Unable to determine internal IP: %s", err)
		}
	}

	igd, err := igdman.NewIGD()
	if err != nil {
		return fmt.Errorf("Unable to get IGD: %s", err)
	}

	igd.RemovePortMapping(igdman.TCP, port)
	err = igd.AddPortMapping(igdman.TCP, internalIP, internalPort, port, 0)
	if err != nil {
		return fmt.Errorf("Unable to map port with igdman %d: %s", port, err)
	}

	return nil
}

func unmapPort(port int) error {
	igd, err := igdman.NewIGD()
	if err != nil {
		return fmt.Errorf("Unable to get IGD: %s", err)
	}

	igd.RemovePortMapping(igdman.TCP, port)
	if err != nil {
		return fmt.Errorf("Unable to unmap port with igdman %d: %s", port, err)
	}

	return nil
}

func determineInternalIP() (string, error) {
	conn, err := net.Dial("tcp", "s3.amazonaws.com:443")
	if err != nil {
		return "", fmt.Errorf("Unable to determine local IP: %s", err)
	}
	defer conn.Close()
	return strings.Split(conn.LocalAddr().String(), ":")[0], nil
}

func (server *Server) startReportingStatsIfNecessary() bool {
	if server.StatReporter != nil {
		log.Debugf("Reporting stats under InstanceId: %s", server.StatReporter.InstanceId)
		go server.StatReporter.Start()
		return true
	} else {
		log.Debug("Not reporting stats (no instanceid specified)")
		return false
	}
}

func (server *Server) startServingStatsIfNecessary() bool {
	if server.StatServer != nil {
		log.Debugf("Serving stats at address: %s", server.StatServer.Addr)
		go server.StatServer.ListenAndServe()
		return true
	} else {
		log.Debug("Not serving stats (no statsaddr specified)")
		return false
	}
}
