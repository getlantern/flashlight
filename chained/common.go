// Package chained provides a chained proxy that can proxy any tcp traffic over
// any underlying transport through a remote proxy. The downstream (client) side
// of the chained setup is just a dial function. The upstream (server) side is
// just an http.Handler. The client tells the server where to connect using an
// HTTP CONNECT request.
package chained

import (
	"strconv"

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
	PluggableTransportSettings map[string]interface{}
}

func (s *ChainedServerInfo) ptSetting(name string) interface{} {
	if s.PluggableTransportSettings == nil {
		return ""
	}
	return s.PluggableTransportSettings[name]
}

func (s *ChainedServerInfo) ptSettingString(name string) string {
	_val := s.ptSetting(name)
	if _val == "" {
		return ""
	}

	f, ok := _val.(string)
	if ok {
		return f
	}
	return ""
}

func (s *ChainedServerInfo) ptSettingInt(name string) int {
	_val := s.ptSetting(name)
	if _val == "" {
		return 0
	}

	f, ok := _val.(int)
	if ok {
		return f
	}

	// For backwards compatibility make sure we still support conversion from
	// strings to ints
	str, oks := _val.(string)
	if oks {
		val, err := strconv.Atoi(str)
		if err != nil {
			log.Errorf("Setting %v: %v is not an int", name, _val)
			return 0
		}
		return val
	}
	return 0
}

func (s *ChainedServerInfo) ptSettingBool(name string) bool {
	_val := s.ptSetting(name)
	if _val == "" {
		return false
	}

	f, ok := _val.(bool)
	if ok {
		return f
	}

	// For backwards compatibility make sure we still support conversion from
	// strings to bools
	str, oks := _val.(string)
	if oks {
		val, err := strconv.ParseBool(str)
		if err != nil {
			log.Errorf("Setting %v: %v is not a bool", name, _val)
			return false
		}
		return val
	}
	return false
}
