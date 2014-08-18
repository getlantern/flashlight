package proxy

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/getlantern/enproxy"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/flashlight/statreporter"
	"github.com/getlantern/flashlight/statserver"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/keyman"
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
	ProxyConfig
	Host                       string                 // FQDN that is guaranteed to hit this server
	CertContext                *CertContext           // context for certificate management
	AllowNonGlobalDestinations bool                   // if true, requests to LAN, Loopback, etc. will be allowed
	StatReporter               *statreporter.Reporter // optional reporter of stats
	StatServer                 *statserver.Server     // optional server of stats
}

// CertContext encapsulates the certificates used by a Server
type CertContext struct {
	PKFile         string
	ServerCertFile string
	pk             *keyman.PrivateKey
	serverCert     *keyman.Certificate
}

func (server *Server) Run() error {
	err := server.CertContext.initServerCert(strings.Split(server.Addr, ":")[0])
	if err != nil {
		return fmt.Errorf("Unable to init server cert: %s", err)
	}

	// Set up an enproxy Proxy
	proxy := &enproxy.Proxy{
		Dial: server.dialDestination,
		Host: server.Host,
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

	tlsConfig := server.TLSConfig
	if server.TLSConfig == nil {
		tlsConfig = DEFAULT_TLS_SERVER_CONFIG
	}
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

// initServerCert initializes a PK + cert for use by a server proxy, signed by
// the CA certificate.  We always generate a new certificate just in case.
func (ctx *CertContext) initServerCert(host string) (err error) {
	if ctx.pk, err = keyman.LoadPKFromFile(ctx.PKFile); err != nil {
		if os.IsNotExist(err) {
			log.Debugf("Creating new PK at: %s", ctx.PKFile)
			if ctx.pk, err = keyman.GeneratePK(2048); err != nil {
				return
			}
			if err = ctx.pk.WriteToFile(ctx.PKFile); err != nil {
				return fmt.Errorf("Unable to save private key: %s", err)
			}
		} else {
			return fmt.Errorf("Unable to read private key, even though it exists: %s", err)
		}
	}

	log.Debugf("Creating new server cert at: %s", ctx.ServerCertFile)
	ctx.serverCert, err = ctx.pk.TLSCertificateFor("Lantern", host, TEN_YEARS_FROM_TODAY, true, nil)
	if err != nil {
		return
	}
	err = ctx.serverCert.WriteToFile(ctx.ServerCertFile)
	if err != nil {
		return
	}
	return nil
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
