package proxy

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/getlantern/enproxy"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/keyman"
)

type Server struct {
	ProxyConfig
	InstanceId         string       // (optional) instanceid under which to report statistics
	CertContext        *CertContext // context for certificate management
	proxy              *enproxy.Proxy
	bytesGivenCh       chan int  // tracks bytes given
	checkpointCh       chan bool // used to sychronize checkpointing of stats to statshub
	checkpointResultCh chan int
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

	server.proxy = enproxy.NewProxy(0, 0)

	// TODO: reenable stats reporting
	server.startReportingStatsIfNecessary()

	// TODO: handle info requests?

	httpServer := &http.Server{
		Addr:         server.Addr,
		Handler:      server,
		ReadTimeout:  server.ReadTimeout,
		WriteTimeout: server.WriteTimeout,
		TLSConfig:    server.TLSConfig,
	}

	if httpServer.TLSConfig == nil {
		httpServer.TLSConfig = DEFAULT_TLS_SERVER_CONFIG
	}

	log.Debugf("About to start server (https) proxy at %s", server.Addr)
	return httpServer.ListenAndServeTLS(server.CertContext.ServerCertFile, server.CertContext.PKFile)
}

func (server *Server) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	log.Debugf("Handling traffic to: %s", req.Header.Get(enproxy.X_HTTPCONN_DEST_ADDR))
	server.proxy.ServeHTTP(resp, req)
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
	if ctx.serverCert, err = ctx.certificateFor(host, TEN_YEARS_FROM_TODAY, true, nil); err != nil {
		return
	}
	if err = ctx.serverCert.WriteToFile(ctx.ServerCertFile); err != nil {
		return
	}
	return nil
}
