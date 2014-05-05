package protocol

import (
	"net"
	"net/http"
)

// a Protocol defines how requests are manipulated on a proxy (both client and
// server)
type Protocol interface {
	// rewriteRequest rewrites an http request
	RewriteRequest(*http.Request)

	// rewriteResponse rewrites an http response
	RewriteResponse(*http.Response)
}

// a ClientProtocol defines how connections are made to the upstream server
type ClientProtocol interface {
	Protocol

	// dial is used on the client to dial out to the upstream server
	Dial(addr string) (net.Conn, error)
}
