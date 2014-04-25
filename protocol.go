package main

import (
	"net"
	"net/http"
)

// a protocol defines how requests are manipulated on a proxy (both client and
// server)
type protocol interface {
	// rewriteRequest rewrites an http request
	rewriteRequest(*http.Request)

	// rewriteResponse rewrites an http response
	rewriteResponse(*http.Response)
}

// a clientProtocol defines how connections are made to the upstream server
type clientProtocol interface {
	protocol

	// dial is used on the client to dial out to the upstream server
	dial(addr string) (net.Conn, error)
}
