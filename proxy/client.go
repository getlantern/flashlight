package proxy

import (
	"net"
	"net/http"
	"strings"

	"github.com/getlantern/enproxy"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/go-reverseproxy/rp"
)

type Client struct {
	ProxyConfig

	EnproxyConfig       *enproxy.Config
	ShouldProxyLoopback bool // if true, even requests to the loopback interface are sent to the server proxy

	reverseProxy *rp.ReverseProxy
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
	if req.Method == CONNECT {
		client.EnproxyConfig.Intercept(resp, req)
	} else {
		client.reverseProxy.ServeHTTP(resp, req)
	}
}

// buildReverseProxy builds the httputil.ReverseProxy used by the client to
// proxy requests upstream.
func (client *Client) buildReverseProxy() {
	client.reverseProxy = &rp.ReverseProxy{
		Director: func(req *http.Request) {
			// do nothing
		},
		Transport: withDumpHeaders(
			client.ShouldDumpHeaders,
			&http.Transport{
				Dial: func(network, addr string) (net.Conn, error) {
					// Check for local addresses, which we proxy directly
					if !client.ShouldProxyLoopback && isLoopback(addr) {
						return net.Dial(network, addr)
					}
					conn := &enproxy.Conn{
						Addr:   addr,
						Config: client.EnproxyConfig,
					}
					err := conn.Connect()
					if err != nil {
						return nil, err
					}
					return conn, nil
				},
			}),
		DynamicFlushInterval: flushIntervalFor,
	}
}

func isLoopback(addr string) bool {
	ip, err := net.ResolveIPAddr("ip4", strings.Split(addr, ":")[0])
	return err == nil && ip.IP.IsLoopback()
}
