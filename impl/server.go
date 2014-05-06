package impl

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/getlantern/flashlight/protocol"
	"github.com/getlantern/keyman"
)

type Server struct {
	ProxyConfig
	Protocol           protocol.Protocol // host-spoofing protocol to use (e.g. CloudFlare)
	InstanceId         string            // (optional) instanceid under which to report statistics
	bytesGivenChan     chan int
	checkpointCh       chan bool
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

	server.reverseProxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			server.Protocol.RewriteRequest(req)
			log.Printf("Handling request for: %s", req.URL.String())
			if server.ShouldDumpHeaders {
				dumpHeaders("Request", req.Header)
			}
		},
		Transport: &http.Transport{
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
			TLSClientConfig: &tls.Config{
				// Use a TLS session cache to minimize TLS connection establishment
				// Requires Go 1.3+
				ClientSessionCache: tls.NewLRUClientSessionCache(TLS_SESSIONS_TO_CACHE_SERVER),
			},
		},
	}

	if server.ShouldDumpHeaders {
		server.reverseProxy.Transport = withRewrite(server.Protocol.RewriteResponse, server.reverseProxy.Transport)
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
// the CA certificate.
func (ctx *CertContext) initServerCert(host string) (err error) {
	ctx.serverCert, err = keyman.LoadCertificateFromFile(ctx.ServerCertFile)
	if err != nil || ctx.serverCert.X509().NotAfter.Before(ONE_MONTH_FROM_TODAY) {
		if err == nil || os.IsNotExist(err) {
			log.Printf("Creating new server cert at: %s", ctx.ServerCertFile)
			if ctx.serverCert, err = ctx.certificateFor(host, ONE_YEAR_FROM_TODAY, true, ctx.caCert); err != nil {
				return
			}
			if err = ctx.serverCert.WriteToFile(ctx.ServerCertFile); err != nil {
				return
			}
		} else {
			return fmt.Errorf("Unable to read server cert, even though it exists: %s", err)
		}
	}
	return nil
}
