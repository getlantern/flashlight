// Package chained provides a chained proxy that can proxy any tcp traffic over
// any underlying transport through a remote proxy. The downstream (client) side
// of the chained setup is just a dial function. The upstream (server) side is
// just an http.Handler. The client tells the server where to connect using an
// HTTP CONNECT request.
package chained

import (
	"strconv"

	"github.com/getlantern/common"
	"github.com/getlantern/golog"
	tls "github.com/refraction-networking/utls"
)

var (
	log = golog.LoggerFor("chained")
)

// ChainedServerInfo contains all the data for connecting to a given chained
// server.
type ChainedServerInfo common.ChainedServerInfo

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
	var ciphers []uint16

	for _, cipherName := range cipherNames {
		cipher, found := availableTLSCiphers[cipherName]
		if !found {
			log.Errorf("Unknown cipher: %v", cipherName)
			continue
		}
		ciphers = append(ciphers, cipher)
	}

	return ciphers
}

func (s *ChainedServerInfo) clientHelloID() tls.ClientHelloID {
	chid := availableClientHelloIDs[s.TLSClientHelloID]
	if chid.Client == "" {
		chid = tls.HelloGolang
	}
	return chid
}

var availableTLSCiphers = map[string]uint16{
	"TLS_RSA_WITH_RC4_128_SHA":                tls.TLS_RSA_WITH_RC4_128_SHA,
	"TLS_RSA_WITH_3DES_EDE_CBC_SHA":           tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	"TLS_RSA_WITH_AES_128_CBC_SHA":            tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	"TLS_RSA_WITH_AES_256_CBC_SHA":            tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	"TLS_RSA_WITH_AES_128_CBC_SHA256":         tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
	"TLS_RSA_WITH_AES_128_GCM_SHA256":         tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	"TLS_RSA_WITH_AES_256_GCM_SHA384":         tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_ECDSA_WITH_RC4_128_SHA":        tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_RC4_128_SHA":          tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
	"TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA":     tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305":    tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305":  tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
}

// helloBrowser is a special hello ID denoting that ClientHellos should be based on those used by
// the system default browser. This structure does not actually get passed to utls code. It is
// caught by tlsConfigForProxy and converted to tls.HelloCustom with a proper corresponding
// ClientHelloSpec.
var helloBrowser = tls.ClientHelloID{
	Client:  "Browser",
	Version: "0",
}

var availableClientHelloIDs = map[string]tls.ClientHelloID{
	"HelloGolang":           tls.HelloGolang,
	"HelloRandomized":       tls.HelloRandomized,
	"HelloRandomizedALPN":   tls.HelloRandomizedALPN,
	"HelloRandomizedNoALPN": tls.HelloRandomizedNoALPN,
	"HelloFirefox_Auto":     tls.HelloFirefox_Auto,
	"HelloFirefox_55":       tls.HelloFirefox_55,
	"HelloFirefox_56":       tls.HelloFirefox_56,
	"HelloChrome_Auto":      tls.HelloChrome_Auto,
	"HelloChrome_58":        tls.HelloChrome_58,
	"HelloChrome_62":        tls.HelloChrome_62,
	"HelloBrowser":          helloBrowser,
}
