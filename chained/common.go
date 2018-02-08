// Package chained provides a chained proxy that can proxy any tcp traffic over
// any underlying transport through a remote proxy. The downstream (client) side
// of the chained setup is just a dial function. The upstream (server) side is
// just an http.Handler. The client tells the server where to connect using an
// HTTP CONNECT request.
package chained

import (
	"crypto/tls"
	"strconv"

	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("chained")

	cipherLookup = map[string]uint16{
		"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305":    tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305":  tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		"TLS_RSA_WITH_AES_128_GCM_SHA256":         tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		"TLS_RSA_WITH_AES_256_GCM_SHA384":         tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		"TLS_RSA_WITH_AES_128_CBC_SHA":            tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		"TLS_RSA_WITH_AES_256_CBC_SHA":            tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		"TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA":     tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
		"TLS_RSA_WITH_3DES_EDE_CBC_SHA":           tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	}
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

	// PluggableTransport: If specified, a pluggable transport will be used
	PluggableTransport string

	// PluggableTransportSettings: Settings for pluggable transport
	PluggableTransportSettings map[string]string

	// KCPSettings: If specified, traffic will be tunneled over KCP
	KCPSettings map[string]interface{}

	// The URL at which to access a domain-fronting server farm using the enhttp
	// protocol.
	ENHTTPURL string

	// DesktopOrderedCipherSuiteNames: The ordered list of cipher suites to use
	// for desktop clients using TLS represented as strings.
	DesktopOrderedCipherSuiteNames []string

	// OrderedMobileCipherSuiteNames: The ordered list of cipher suites to use for
	// mobile clients using TLS represented as strings. This may differ from the
	// ordering for desktop because performance of AES ciphers is more of a
	// concern on mobile.
	MobileOrderedCipherSuiteNames []string
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
	return ciphersFromNames(s.DesktopOrderedCipherSuiteNames)
}

func (s *ChainedServerInfo) mobileOrderedCipherSuites() []uint16 {
	return ciphersFromNames(s.MobileOrderedCipherSuiteNames)
}

func ciphersFromNames(cipherNames []string) []uint16 {
	var ciphers []uint16

	for _, name := range cipherNames {
		ciphers = append(ciphers, cipherLookup[name])
	}

	return ciphers
}
