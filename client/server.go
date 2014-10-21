package client

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/getlantern/connpool"
	"github.com/getlantern/enproxy"
	"github.com/getlantern/flashlight/proxy"
	"github.com/getlantern/keyman"
	"net/http/httputil"

	"gopkg.in/getlantern/tlsdialer.v1"
)

type server struct {
	info          *ServerInfo
	masquerades   *verifiedMasqueradeSet
	enproxyConfig *enproxy.Config
	connPool      *connpool.Pool
	reverseProxy  *httputil.ReverseProxy
}

// buildReverseProxy builds the httputil.ReverseProxy used to proxy requests to
// the server.
func (server *server) buildReverseProxy(shouldDumpHeaders bool) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// do nothing
		},
		Transport: withDumpHeaders(
			shouldDumpHeaders,
			&http.Transport{
				// We disable keepalives because some servers pretend to support
				// keep-alives but close their connections immediately, which
				// causes an error inside ReverseProxy.  This is not an issue
				// for HTTPS because  the browser is responsible for handling
				// the problem, which browsers like Chrome and Firefox already
				// know to do.
				// See https://code.google.com/p/go/issues/detail?id=4677
				DisableKeepAlives: true,
				Dial:              server.dialWithEnproxy,
			}),
		// Set a FlushInterval to prevent overly aggressive buffering of
		// responses, which helps keep memory usage down
		FlushInterval: 250 * time.Millisecond,
	}
}

func (server *server) dialWithEnproxy(network, addr string) (net.Conn, error) {
	conn := &enproxy.Conn{
		Addr:   addr,
		Config: server.enproxyConfig,
	}
	err := conn.Connect()
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (server *server) buildEnproxyConfig() *enproxy.Config {
	server.connPool = &connpool.Pool{
		MinSize:      30,
		ClaimTimeout: 15 * time.Second,
		Dial:         server.info.dialerFor(server.nextMasquerade),
	}
	server.connPool.Start()

	return server.info.enproxyConfigWith(func(addr string) (net.Conn, error) {
		return server.connPool.Get()
	})
}

func (server *server) close() {
	if server.connPool != nil {
		server.connPool.Stop()
	}
}

func (server *server) nextMasquerade() *Masquerade {
	if server.masquerades == nil {
		return nil
	}
	masquerade := server.masquerades.nextVerified()
	return masquerade
}

// withDumpHeaders creates a RoundTripper that uses the supplied RoundTripper
// and that dumps headers is client is so configured.
func withDumpHeaders(shouldDumpHeaders bool, rt http.RoundTripper) http.RoundTripper {
	if !shouldDumpHeaders {
		return rt
	}
	return &headerDumpingRoundTripper{rt}
}

// headerDumpingRoundTripper is an http.RoundTripper that wraps another
// http.RoundTripper and dumps response headers to the log.
type headerDumpingRoundTripper struct {
	orig http.RoundTripper
}

func (rt *headerDumpingRoundTripper) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	proxy.DumpHeaders("Request", &req.Header)
	resp, err = rt.orig.RoundTrip(req)
	if err == nil {
		proxy.DumpHeaders("Response", &resp.Header)
	}
	return
}

// buildServer builds a server configured from this serverInfo using the given
// enproxy.Config if provided.
func (serverInfo *ServerInfo) buildServer(shouldDumpHeaders bool, masquerades *verifiedMasqueradeSet, enproxyConfig *enproxy.Config) *server {
	weight := serverInfo.Weight
	if weight == 0 {
		weight = 100
	}

	server := &server{
		info:          serverInfo,
		masquerades:   masquerades,
		enproxyConfig: enproxyConfig,
	}

	if server.enproxyConfig == nil {
		// Build a dynamic config
		server.enproxyConfig = server.buildEnproxyConfig()
	}
	server.reverseProxy = server.buildReverseProxy(shouldDumpHeaders)

	return server
}

// disposableEnproxyConfig creates an enproxy.Config for one-time use (no
// pooling, etc.)
func (serverInfo *ServerInfo) disposableEnproxyConfig(masquerade *Masquerade) *enproxy.Config {
	masqueradeSource := func() *Masquerade { return masquerade }
	dial := serverInfo.dialerFor(masqueradeSource)
	dialFunc := func(addr string) (net.Conn, error) {
		return dial()
	}
	return serverInfo.enproxyConfigWith(dialFunc)
}

func (serverInfo *ServerInfo) enproxyConfigWith(dialProxy func(addr string) (net.Conn, error)) *enproxy.Config {
	return &enproxy.Config{
		DialProxy: dialProxy,
		NewRequest: func(upstreamHost string, method string, body io.Reader) (req *http.Request, err error) {
			if upstreamHost == "" {
				// No specific host requested, use configured one
				upstreamHost = serverInfo.Host
			}
			return http.NewRequest(method, "http://"+upstreamHost+"/", body)
		},
		BufferRequests: serverInfo.BufferRequests,
	}
}

func (serverInfo *ServerInfo) dialerFor(masqueradeSource func() *Masquerade) func() (net.Conn, error) {
	dialTimeout := time.Duration(serverInfo.DialTimeoutMillis) * time.Millisecond
	if dialTimeout == 0 {
		dialTimeout = 5 * time.Second
	}

	// Note - we need to suppress the sending of the ServerName in the
	// client handshake to make host-spoofing work with Fastly.  If the
	// client Hello includes a server name, Fastly checks to make sure
	// that this matches the Host header in the HTTP request and if they
	// don't match, it returns a 400 Bad Request error.
	sendServerNameExtension := false

	return func() (net.Conn, error) {
		masquerade := masqueradeSource()
		return tlsdialer.DialWithDialer(
			&net.Dialer{
				Timeout: dialTimeout,
			},
			"tcp",
			serverInfo.addressForServer(masquerade),
			sendServerNameExtension,
			serverInfo.tlsConfig(masquerade))
	}
}

// Get the address to dial for reaching the server
func (serverInfo *ServerInfo) addressForServer(masquerade *Masquerade) string {
	return fmt.Sprintf("%s:%d", serverInfo.serverHost(masquerade), serverInfo.Port)
}

func (serverInfo *ServerInfo) serverHost(masquerade *Masquerade) string {
	serverHost := serverInfo.Host
	if masquerade != nil && masquerade.Domain != "" {
		serverHost = masquerade.Domain
	}
	return serverHost
}

// Build a tls.Config for dialing the upstream host
func (serverInfo *ServerInfo) tlsConfig(masquerade *Masquerade) *tls.Config {
	tlsConfig := &tls.Config{
		ClientSessionCache: tls.NewLRUClientSessionCache(1000),
		InsecureSkipVerify: serverInfo.InsecureSkipVerify,
	}

	if masquerade != nil && masquerade.RootCA != "" {
		caCert, err := keyman.LoadCertificateFromPEMBytes([]byte(masquerade.RootCA))
		if err != nil {
			log.Fatalf("Unable to load root ca cert: %s", err)
		}
		tlsConfig.RootCAs = caCert.PoolContainingCert()
	}
	return tlsConfig
}
