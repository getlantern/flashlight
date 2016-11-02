package client

import (
	"context"
	"net"
	"net/http"

	"github.com/getlantern/errors"
	"github.com/getlantern/netx"
)

// Proxy represents a proxy Lantern client can connect to.
type Proxy interface {
	// How can we dial this proxy
	netx.Dialer
	// How can we proxy a http(s) request over it
	http.RoundTripper
	// A human friendly string to identify the proxy
	Label() string
	// Check the reachibility of the proxy
	Check() bool
	// Set extra headers sent along with each proxy request.
	SetExtraHeaders(headers http.Header)
	// Clean up resources, if any
	Close()
}

// CreateProxy creates a Proxy with supplied server info, or returns the error
func CreateProxy(s *ChainedServerInfo) (Proxy, error) {
	switch s.PluggableTransport {
	case "":
		return httpsOverTCP{}, nil
	case "obfs4-tcp":
		return obfs4OverTCP{}, nil
	case "obfs4-kcp":
		return obfs4OverKCP{}, nil
	default:
		return nil, errors.New("Unknown transport").With("plugabble-transport", s.PluggableTransport)
	}
}

type httpsOverTCP struct {
	baseProxy
}

type obfs4OverTCP struct {
	baseProxy
}

type obfs4OverKCP struct {
	baseProxy
}

type nullDialer struct{}

func (d nullDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	panic("should implement DialContext")
	return nil, nil
}

type baseProxy struct {
	netx.Dialer
	http.RoundTripper
	extraHeaders http.Header
}

func (p baseProxy) Label() string {
	return "null"
}

func (p baseProxy) Check() bool {
	return false
}

func (p baseProxy) SetExtraHeaders(headers http.Header) {
	p.extraHeaders = headers
}

func (p baseProxy) Close() {
}
