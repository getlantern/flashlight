package client

import (
	"net"
	"net/http"
)

type directConn struct {
	net.Conn
}

// OnRequest implements proxy.RequestAware
func (conn *directConn) OnRequest(req *http.Request) {
	// If we're sending traffic directly rather than via a proxy, we want to delete
	// the Proxy-Connection header and convert it to a Connection header, which
	// is what the browser expects.
	pc := req.Header.Get("Proxy-Connection")
	if pc != "" {
		req.Header.Set("Connection", pc)
	}
	req.Header.Del("Proxy-Connection")
}
