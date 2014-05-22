package proxy

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/getlantern/flashlight/protocol"
	"github.com/getlantern/go-reverseproxy/rp"
)

type Server struct {
	ProxyConfig
	Protocol           protocol.Protocol // host-spoofing protocol to use (e.g. CloudFlare)
	InstanceId         string            // (optional) instanceid under which to report statistics
	TLSClientConfig    *tls.Config       // (optional) configuration for TLS client used for outbound connections
	bytesGivenCh       chan int          // tracks bytes given
	checkpointCh       chan bool         // used to sychronize checkpointing of stats to statshub
	checkpointResultCh chan int
}

func (server *Server) Run() error {
	err := server.CertContext.InitCommonCerts()
	if err != nil {
		return fmt.Errorf("Unable to init common certs: %s", err)
	}

	err = server.CertContext.initServerCert(strings.Split(server.Addr, ":")[0])
	if err != nil {
		return fmt.Errorf("Unable to init server cert: %s", err)
	}

	server.buildReverseProxy()

	server.startReportingStatsIfNecessary()

	httpServer := &http.Server{
		Addr:         server.Addr,
		Handler:      http.HandlerFunc(server.handleServer),
		ReadTimeout:  server.ReadTimeout,
		WriteTimeout: server.WriteTimeout,
		TLSConfig:    server.TLSConfig,
	}
	if httpServer.TLSConfig == nil {
		httpServer.TLSConfig = DEFAULT_TLS_SERVER_CONFIG
	}

	log.Printf("About to start server (https) proxy at %s", server.Addr)
	return httpServer.ListenAndServeTLS(server.CertContext.ServerCertFile, server.CertContext.PKFile)
}

func (server *Server) buildReverseProxy() {
	shouldReportStats := server.InstanceId != ""

	tlsClientConfig := &tls.Config{}
	if server.TLSClientConfig != nil {
		// Make a copy of the supplied config
		*tlsClientConfig = *server.TLSClientConfig
	}
	// Use a TLS session cache to minimize TLS connection establishment
	// Requires Go 1.3+
	tlsClientConfig.ClientSessionCache = tls.NewLRUClientSessionCache(TLS_SESSIONS_TO_CACHE_SERVER)

	server.reverseProxy = &rp.ReverseProxy{
		Director: func(req *http.Request) {
			server.Protocol.RewriteRequest(req)
			log.Printf("Handling request for: %s", req.URL.String())
			if server.ShouldDumpHeaders {
				dumpHeaders("Request", req.Header)
			}
		},
		Transport: withRewrite(
			server.Protocol.RewriteResponse,
			server.ShouldDumpHeaders,
			&http.Transport{
				Dial: func(network, addr string) (net.Conn, error) {
					conn, err := net.Dial(network, addr)
					if err != nil {
						return nil, err
					}
					if shouldReportStats {
						// When reporting stats, use a special connection that counts bytes
						return &countingConn{conn, server}, nil
					}
					return conn, err
				},
				TLSClientConfig: tlsClientConfig,
			}),
		DynamicFlushInterval: flushIntervalFor,
	}
}

// handleServer handles requests to the server-side (upstream) proxy
func (server *Server) handleServer(resp http.ResponseWriter, req *http.Request) {
	if req.Header.Get(X_LANTERN_REQUEST_INFO) != "" {
		server.handleInfoRequest(resp, req)
	} else {
		// Proxy as usual
		server.reverseProxy.ServeHTTP(resp, req)
	}
}

// handleInfoRequest looks up info about the client (right now just ip address)
// and returns it to the client
func (server *Server) handleInfoRequest(resp http.ResponseWriter, req *http.Request) {
	// Client requested their info
	clientIp := req.Header.Get("X-Forwarded-For")
	if clientIp == "" {
		clientIp = strings.Split(req.RemoteAddr, ":")[0]
	} else {
		// X-Forwarded-For may contain multiple ips, use the last
		ips := strings.Split(clientIp, ",")
		clientIp = ips[len(ips)-1]
	}
	resp.Header().Set(X_LANTERN_PUBLIC_IP, clientIp)
	resp.WriteHeader(200)
}

// initServerCert initializes a certificate for use by a server proxy, signed by
// the CA certificate.  We always generate a new certificate just in case.
func (ctx *CertContext) initServerCert(host string) (err error) {
	log.Printf("Creating new server cert at: %s", ctx.ServerCertFile)
	if ctx.serverCert, err = ctx.certificateFor(host, TEN_YEARS_FROM_TODAY, true, ctx.caCert); err != nil {
		return
	}
	if err = ctx.serverCert.WriteToFile(ctx.ServerCertFile); err != nil {
		return
	}
	return nil
}
