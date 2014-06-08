package proxy

import (
	"net"
	"net/http"
	"net/http/httputil"

	"github.com/getlantern/enproxy"
	"github.com/getlantern/flashlight/log"
)

const (
	CONNECT = "CONNECT" // HTTP CONNECT method
)

type Client struct {
	ProxyConfig

	EnproxyConfig *enproxy.Config

	reverseProxy *httputil.ReverseProxy
}

func (client *Client) Run() error {
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
				Dial: func(network, addr string) (net.Conn, error) {
					conn := &enproxy.Conn{
						Addr:   addr,
						Config: client.EnproxyConfig,
					}
					conn.Connect()
					return conn, nil
				},
			}),
	}
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
	dumpHeaders("Request", req.Header)
	resp, err = rt.orig.RoundTrip(req)
	if err == nil {
		dumpHeaders("Response", resp.Header)
	}
	return
}
