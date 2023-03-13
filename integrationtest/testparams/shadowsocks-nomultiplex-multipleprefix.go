package testparams

import (
	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/testcases"
	"github.com/getlantern/flashlight-integration-test/util"
)

var Test_Shadowsocks_NoMultiplex_MultiplePrefix TestParams

func init() {
	Test_Shadowsocks_NoMultiplex_MultiplePrefix = TestParams{
		Name: "shadowsocks-no-multiplex-multiple-prefix",
		ProxyConfig: &config.ProxyConfig{
			PluggableTransport: "shadowsocks",
			PluggableTransportSettings: map[string]string{
				"shadowsocks_secret": "secret",
				"shadowsocks_cipher": "chacha20-ietf-poly1305",
			},
			Addr:      DefaultTestAddr,
			AuthToken: "bunnyfoofoo",
			Cert: string(
				util.MustReadFile(LocalHTTPProxyLanternTestCertFile),
			),
			Prefixes: []string{
				// Not necessary, but for this test, we want to make sure all
				// prefixes are the same length since we're hardcoding the
				// prefix size in the http-proxy-lantern proxy below.
				"41414141", // hex representation of 4 bytes: AAAA
				"42424242", // hex representation of 4 bytes: BBBB
				"43434343", // hex representation of 4 bytes: CCCC
			},
		},
		TestCases: testcases.DefaultTestCases,
	}
}
