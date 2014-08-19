package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/getlantern/enproxy"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/keyman"
	"github.com/getlantern/tls"
)

const (
	CONNECT = "CONNECT" // HTTP CONNECT method

	REVERSE_PROXY_FLUSH_INTERVAL = 250 * time.Millisecond
)

type Client struct {
	ProxyConfig
	UpstreamHost       string
	UpstreamPort       int
	MasqueradeAs       string
	RootCA             string
	InsecureSkipVerify bool
	EnproxyConfig      *enproxy.Config

	reverseProxy *httputil.ReverseProxy
}

func (client *Client) Run() error {
	if client.EnproxyConfig == nil {
		client.BuildEnproxyConfig()
	}
	client.buildReverseProxy()

	httpServer := &http.Server{
		Addr:         client.Addr,
		ReadTimeout:  client.ReadTimeout,
		WriteTimeout: client.WriteTimeout,
		Handler:      client,
	}

	log.Debugf("About to start client (http) proxy at %s", client.Addr)
	return httpServer.ListenAndServe()
}

func (client *Client) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	log.Debugf("Handling request for: %s", req.RequestURI)
	if req.Method == CONNECT {
		client.EnproxyConfig.Intercept(resp, req)
	} else {
		client.reverseProxy.ServeHTTP(resp, req)
	}
}

func (client *Client) BuildEnproxyConfig() {
	client.EnproxyConfig = &enproxy.Config{
		DialProxy: func(addr string) (net.Conn, error) {
			return tls.DialWithDialer(
				&net.Dialer{
					Timeout:   20 * time.Second,
					KeepAlive: 70 * time.Second,
				},
				"tcp", client.addressForServer(), client.tlsConfig())
		},
		NewRequest: func(host string, method string, body io.Reader) (req *http.Request, err error) {
			if host == "" {
				host = client.UpstreamHost
			}
			return http.NewRequest(method, "http://"+host+"/", body)
		},
	}
}

func (client *Client) DialWithEnproxy(network, addr string) (net.Conn, error) {
	conn := &enproxy.Conn{
		Addr:   addr,
		Config: client.EnproxyConfig,
	}
	err := conn.Connect()
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// buildReverseProxy builds the httputil.ReverseProxy used by the client to
// proxy requests upstream.
func (client *Client) buildReverseProxy() {
	client.reverseProxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// do nothing
		},
		Transport: withDumpHeaders(
			client.ShouldDumpHeaders,
			&http.Transport{
				// We disable keepalives because some servers pretend to support
				// keep-alives but close their connections immediately, which
				// causes an error inside ReverseProxy.  This is not an issue
				// for HTTPS because  the browser is responsible for handling
				// the problem, which browsers like Chrome and Firefox already
				// know to do.
				// See https://code.google.com/p/go/issues/detail?id=4677
				DisableKeepAlives: true,
				Dial:              client.DialWithEnproxy,
			}),
		// Set a FlushInterval to prevent overly aggressive buffering of
		// responses, which helps keep memory usage down
		FlushInterval: 250 * time.Millisecond,
	}
}

// Get the address to dial for reaching the server
func (client *Client) addressForServer() string {
	serverHost := client.UpstreamHost
	if client.MasqueradeAs != "" {
		serverHost = client.MasqueradeAs
	}
	return fmt.Sprintf("%s:%d", serverHost, client.UpstreamPort)
}

// Build a tls.Config for the client to use in dialing server
func (client *Client) tlsConfig() *tls.Config {
	tlsConfig := &tls.Config{
		ClientSessionCache:                  tls.NewLRUClientSessionCache(1000),
		SuppressServerNameInClientHandshake: true,
		InsecureSkipVerify:                  client.InsecureSkipVerify,
	}
	// Note - we need to suppress the sending of the ServerName in the client
	// handshake to make host-spoofing work with Fastly.  If the client Hello
	// includes a server name, Fastly checks to make sure that this matches the
	// Host header in the HTTP request and if they don't match, it returns a
	// 400 Bad Request error.
	if client.RootCA != "" {
		caCert, err := keyman.LoadCertificateFromPEMBytes([]byte(client.RootCA))
		if err != nil {
			log.Fatalf("Unable to load root ca cert: %s", err)
		}
		tlsConfig.RootCAs = caCert.PoolContainingCert()
	}
	return tlsConfig
}

// withDumpHeaders creates a RoundTripper that uses the supplied RoundTripper
// and that dumps headers (if dumpHeaders is true).
func withDumpHeaders(dumpHeaders bool, rt http.RoundTripper) http.RoundTripper {
	if !dumpHeaders {
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
	dumpHeaders("Request", &req.Header)
	resp, err = rt.orig.RoundTrip(req)
	if err == nil {
		dumpHeaders("Response", &resp.Header)
	}
	return
}
