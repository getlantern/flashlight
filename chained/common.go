// Package chained provides a chained proxy that can proxy any tcp traffic over
// any underlying transport through a remote proxy. The downstream (client) side
// of the chained setup is just a dial function. The upstream (server) side is
// just an http.Handler. The client tells the server where to connect using an
// HTTP CONNECT request.
package chained

import (
	"reflect"

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

func (a *ChainedServerInfo) Equals(b *ChainedServerInfo) bool {
	if a.Addr != b.Addr {
		return false
	}
	if a.Cert != b.Cert {
		return false
	}
	if a.AuthToken != b.AuthToken {
		return false
	}
	pta := a.PluggableTransport
	ptb := b.PluggableTransport
	if pta == "obfs4" {
		pta = "obfs4-tcp"
	}
	if pta == "obfs4" {
		ptb = "obfs4-tcp"
	}
	if pta != ptb {
		return false
	}

	if a.PluggableTransportSettings == nil {
		return b.PluggableTransportSettings != nil
	} else if b.PluggableTransportSettings == nil {
		return false
	}

	return reflect.DeepEqual(a.PluggableTransportSettings, b.PluggableTransportSettings)
}
