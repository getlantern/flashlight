// Package chained provides a chained proxy that can proxy any tcp traffic over
// any underlying transport through a remote proxy. The downstream (client) side
// of the chained setup is just a dial function. The upstream (server) side is
// just an http.Handler. The client tells the server where to connect using an
// HTTP CONNECT request.
package chained

import (
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("chained")
)

// ChainedServerInfo contains all the data for connecting to a given chained
// server.
type ChainedServerInfo struct {
	// Addr: the host:port of the upstream proxy server
	Addr string

	// Cert: optional PEM encoded certificate for the server. If specified,
	// server will be dialed using TLS over tcp. Otherwise, server will be
	// dialed using plain tcp. For OBFS4 proxies, this is the Base64-encoded obfs4
	// certificate.
	Cert string

	// AuthToken: the authtoken to present to the upstream server.
	AuthToken string

	// Trusted: Determines if a host can be trusted with plain HTTP traffic.
	Trusted bool

	// PluggableTransport: If specified, a pluggable transport will be used
	PluggableTransport string

	// PluggableTransportSettings: Settings for pluggable transport
	PluggableTransportSettings map[string]string
}
