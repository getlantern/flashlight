package fastly

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	"github.com/getlantern/flashlight/log"
)

// fastlyServerProtocol implements clientProtocol using Fastly (not yet working)
type FastlyClientProtocol struct {
	upstreamHost string
	fastlyHost   string
	upstreamAddr string
}

func NewClientProtocol(upstreamHost string, upstreamPort int, masqueradeAs string) *FastlyClientProtocol {
	fastlyHost := upstreamHost
	if masqueradeAs != "" {
		fastlyHost = masqueradeAs
	}
	return &FastlyClientProtocol{
		upstreamHost: upstreamHost,
		fastlyHost:   fastlyHost,
		upstreamAddr: fmt.Sprintf("%s:%d", fastlyHost, upstreamPort),
	}
}

func (cf *FastlyClientProtocol) RewriteRequest(req *http.Request) {
	// Remember the host and scheme that was actually requested
	req.Header.Set(X_LANTERN_URL, req.URL.String())
	req.URL.Scheme = "http"

	// Set our upstream proxy as the host for this request
	req.Host = cf.upstreamHost
	req.URL.Host = cf.upstreamHost
}

func (cf *FastlyClientProtocol) RewriteResponse(resp *http.Response) {
}

func (cf *FastlyClientProtocol) Dial(addr string) (net.Conn, error) {
	log.Debugf("Using %s to handle request", cf.upstreamAddr)

	// Manually dial and upgrade to TLS to avoid logic in tls.Dial() that
	// defaults the ServerName based on the host being dialed.
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	conn := tls.Client(c, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err = conn.Handshake(); err != nil {
		c.Close()
		return nil, err
	}
	return conn, nil
}
