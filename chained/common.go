// Package chained provides a chained proxy that can proxy any tcp traffic over
// any underlying transport through a remote proxy. The downstream (client) side
// of the chained setup is just a dial function. The upstream (server) side is
// just an http.Handler. The client tells the server where to connect using an
// HTTP CONNECT request.
package chained

import (
	"go/importer"
	"go/types"
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

	// InitPreconnect: how much to preconnect on startup
	InitPreconnect int

	// MaxPreconnect: the maximum number of preconnections to keep
	MaxPreconnect int

	// Bias indicates a relative biasing factor for proxy selection purposes.
	// Proxies are bias 0 by default, meaning that they're prioritized by the
	// usual bandwidth and latency metrics. Proxies with a higher bias are
	// preferred over proxies with a lower bias irrespective of their measured
	// performance.
	Bias int

	// PluggableTransport: If specified, a pluggable transport will be used
	PluggableTransport string

	// PluggableTransportSettings: Settings for pluggable transport
	PluggableTransportSettings map[string]string

	// KCPSettings: If specified, traffic will be tunneled over KCP
	KCPSettings map[string]interface{}

	// The URL at which to access a domain-fronting server farm using the enhttp
	// protocol.
	ENHTTPURL string

	// TLSDesktopOrderedCipherSuiteNames: The ordered list of cipher suites to use
	// for desktop clients using TLS represented as strings.
	TLSDesktopOrderedCipherSuiteNames []string

	// TLSMobileOrderedCipherSuiteNames: The ordered list of cipher suites to use
	// for mobile clients using TLS represented as strings. This may differ from
	// the ordering for desktop because performance of AES ciphers is more of a
	// concern on mobile.
	TLSMobileOrderedCipherSuiteNames []string

	// TLSServerNameIndicator: Specifies the hostname that should be sent by the
	// client as the Server Name Indication header in a TLS request.  If not
	// provided, the client should not send an SNI header.
	TLSServerNameIndicator string

	// TLSClientSessionCacheSize: the size of client session cache to use. Set to
	// 0 to use default size, set to < 0 to disable.
	TLSClientSessionCacheSize int
}

func (s *ChainedServerInfo) ptSetting(name string) string {
	if s.PluggableTransportSettings == nil {
		return ""
	}
	return s.PluggableTransportSettings[name]
}

func (s *ChainedServerInfo) ptSettingInt(name string) int {
	_val := s.ptSetting(name)
	if _val == "" {
		return 0
	}
	val, err := strconv.Atoi(_val)
	if err != nil {
		log.Errorf("Setting %v: %v is not an int", name, _val)
		return 0
	}
	return val
}

func (s *ChainedServerInfo) ptSettingBool(name string) bool {
	_val := s.ptSetting(name)
	if _val == "" {
		return false
	}
	val, err := strconv.ParseBool(_val)
	if err != nil {
		log.Errorf("Setting %v: %v is not a boolean", name, _val)
		return false
	}
	return val
}

func (s *ChainedServerInfo) desktopOrderedCipherSuites() []uint16 {
	return ciphersFromNames(s.TLSDesktopOrderedCipherSuiteNames)
}

func (s *ChainedServerInfo) mobileOrderedCipherSuites() []uint16 {
	return ciphersFromNames(s.TLSMobileOrderedCipherSuiteNames)
}

func ciphersFromNames(cipherNames []string) []uint16 {
	ciphers := make([]uint16, 0, len(cipherNames))

	pkg, err := importer.Default().Import("crypto/tls")
	if err != nil {
		log.Errorf("Unable to load crypto/tls package to look up ciphers: %v", err)
		return ciphers
	}

	scope := pkg.Scope()
	for _, name := range cipherNames {
		obj := scope.Lookup("TLS_" + name)
		switch t := obj.(type) {
		case *types.Const:
			if t.Exported() && t.Type().String() == "uint16" {
				_val, parseErr := strconv.ParseUint(t.Val().ExactString(), 10, 16)
				if parseErr != nil {
					log.Errorf("Unable to parse cipher suite value for TLS_%v", name, parseErr)
					continue
				}
				val := uint16(_val)
				ciphers = append(ciphers, val)
			}
		default:
			log.Errorf("Unable to find cipher suite TLS_%v", name)
		}
	}

	if len(ciphers) == 0 {
		// Set ciphers to nil so that tls.Config uses default ciphers
		ciphers = nil
	}
	return ciphers
}
