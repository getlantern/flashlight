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
}
