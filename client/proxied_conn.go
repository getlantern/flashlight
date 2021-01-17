package client

import (
	"net"
	"net/http"

	"github.com/getlantern/flashlight/common"
)

type proxiedConn struct {
	net.Conn
}

// OnRequest implements proxy.RequestAware
func (conn *proxiedConn) OnRequest(req *http.Request) {
	// consumed and removed by http-proxy-lantern/versioncheck
	req.Header.Set(common.VersionHeader, common.Version)

	// By default the proxy at this point has swapped any Proxy-Connection headers for Connection.
	// In this chained proxy case, we want to swap it back.
	pc := req.Header.Get("Connection")
	if pc != "" {
		req.Header.Set("Proxy-Connection", pc)
	}
	req.Header.Del("Connection")
}

func (conn *proxiedConn) Wrapped() net.Conn {
	return conn.Conn
}
