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
	"github.com/getlantern/keyman"
)

var (
	dialTimeout = 10 * time.Second

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
	Host                       string       // FQDN that is guaranteed to hit this server
	InstanceId                 string       // (optional) instanceid under which to report statistics
	Country                    string       // (optional) country under which to report statistics
	CertContext                *CertContext // context for certificate management
	AllowNonGlobalDestinations bool         // if true, requests to LAN, Loopback, etc. will be allowed
	bytesGiven                 int64        // tracks bytes given
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
	if server.startReportingStatsIfNecessary() {
		// Add callbacks to track bytes given
		proxy.OnBytesReceived = server.onBytesGiven
		proxy.OnBytesSent = server.onBytesGiven
	}
	proxy.Start()

	httpServer := &http.Server{
		Addr:         server.Addr,
		Handler:      proxy,
		ReadTimeout:  server.ReadTimeout,
		WriteTimeout: server.WriteTimeout,
		// TLSConfig:    server.TLSConfig,
	}
	// TODO: Add flag to reenable this
	// if httpServer.TLSConfig == nil {
	// 	httpServer.TLSConfig = DEFAULT_TLS_SERVER_CONFIG
	// }

	log.Debugf("About to start server (https) proxy at %s", server.Addr)
	// return httpServer.ListenAndServeTLS(server.CertContext.ServerCertFile, server.CertContext.PKFile)
	return httpServer.ListenAndServe()
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
