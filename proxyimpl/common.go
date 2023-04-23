package proxyimpl

import (
	"context"
	"math/rand"
	"runtime"
	"strconv"
	"time"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/browsers/simbrowser"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	tls "github.com/refraction-networking/utls"
)

var (
	ChainedDialTimeout = 1 * time.Minute

	// IdleTimeout closes connections idle for a period to avoid dangling
	// connections. Web applications tend to contact servers in 1 minute
	// interval or below. 65 seconds is long enough to avoid interrupt normal
	// connections but shorter than the idle timeout on the server to avoid
	// running into closed connection problems.
	IdleTimeout = 65 * time.Second

	defaultMultiplexedPhysicalConns int32 = 1
)

func splitClientHello(hello []byte) [][]byte {
	const minSplits, maxSplits = 2, 5
	var (
		maxLen = len(hello) / minSplits
		splits = [][]byte{}
		start  = 0
		end    = start + rand.Intn(maxLen) + 1
	)
	for end < len(hello) && len(splits) < maxSplits-1 {
		splits = append(splits, hello[start:end])
		start = end
		end = start + rand.Intn(maxLen) + 1
	}
	splits = append(splits, hello[start:])
	return splits
}

func clientHelloID(pc *config.ProxyConfig) tls.ClientHelloID {
	chid := availableClientHelloIDs[pc.TLSClientHelloID]
	if chid.Client == "" {
		chid = tls.HelloGolang
	}
	return chid
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
	"HelloFirefox_105":      tls.HelloFirefox_105,
	"HelloChrome_Auto":      tls.HelloChrome_Auto,
	"HelloChrome_58":        tls.HelloChrome_58,
	"HelloChrome_62":        tls.HelloChrome_62,
	"HelloChrome_106":       tls.HelloChrome_106,
	"HelloEdge_Auto":        tls.HelloEdge_Auto,
	"Hello360_Auto":         tls.Hello360_Auto,
	"HelloQQ_Auto":          tls.HelloQQ_Auto,
	"HelloQQ_11":            tls.HelloQQ_11_1,
	"HelloBrowser":          helloBrowser,
}

// getBrowserHello determines the best way to mimic the system's default web browser. There are a
// few possible failure points in making this determination, e.g. a failure to obtain the default
// browser or a failure to capture a hello from the browser. However, this function will always find
// something reasonable to fall back on.
func getBrowserHello(ctx context.Context, configDir string, uc common.UserConfig) helloSpec {
	// We have a number of ways to approximate the browser's ClientHello format. We begin with the
	// most desirable, progressively falling back to less desirable options on failure.

	op := ops.Begin("get_browser_hello")
	op.Set("platform", runtime.GOOS)
	defer op.End()

	hello, err := activelyObtainBrowserHello(ctx, configDir)
	if err == nil {
		return *hello
	}
	op.FailIf(err)
	log.Debugf("failed to actively obtain browser hello: %v", err)

	// Our last option is to simulate a browser choice for the user based on market share.
	return helloSpec{simbrowser.ChooseForUser(ctx, uc).ClientHelloID, nil}
}

func _setting(settings map[string]string, name string) string {
	if settings == nil {
		return ""
	}
	return settings[name]
}

func _settingInt(settings map[string]string, name string) int {
	_val := _setting(settings, name)
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

func _settingFloat(settings map[string]string, name string) float64 {
	_val := _setting(settings, name)
	if _val == "" {
		return 0.0
	}
	val, err := strconv.ParseFloat(_val, 64)
	if err != nil {
		log.Errorf("Setting %v: %v is not a float", name, _val)
		return 0.0
	}
	return val
}

func _settingBool(settings map[string]string, name string) bool {
	_val := _setting(settings, name)
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

func muxSettingInt(pc *config.ProxyConfig, name string) int {
	return _settingInt(pc.MultiplexedSettings, name)
}

func muxSettingBool(pc *config.ProxyConfig, name string) bool {
	return _settingBool(pc.MultiplexedSettings, name)
}

func muxSettingFloat(pc *config.ProxyConfig, name string) float64 {
	return _settingFloat(pc.MultiplexedSettings, name)
}

func ptSettingInt(pc *config.ProxyConfig, name string) int {
	return _settingInt(pc.PluggableTransportSettings, name)
}

func ptSetting(pc *config.ProxyConfig, name string) string {
	return _setting(pc.PluggableTransportSettings, name)
}

func ptSettingBool(pc *config.ProxyConfig, name string) bool {
	return _settingBool(pc.PluggableTransportSettings, name)
}

func desktopOrderedCipherSuites(pc *config.ProxyConfig) []uint16 {
	return ciphersFromNames(pc.TLSDesktopOrderedCipherSuiteNames)
}

func mobileOrderedCipherSuites(pc *config.ProxyConfig) []uint16 {
	return ciphersFromNames(pc.TLSMobileOrderedCipherSuiteNames)
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
