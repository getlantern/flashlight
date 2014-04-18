package main

import (
	"net"
	"net/http"
)

// a protocol defines how requests are manipulated on a proxy (both client and
// server)
type protocol interface {
	// rewrite rewrites an http request
	rewrite(*http.Request)
}

// a clientProtocol defines how connections are made to the upstream server
type clientProtocol interface {
	protocol

	// dial is used on the client to dial out to the upstream server
	dial(addr string) (net.Conn, error)
}
