package testparams

import (
	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/testcases"
	"github.com/getlantern/flashlight-integration-test/util"
)

var Test_Shadowsocks_NoMultiplex_NoPrefix TestParams

func init() {
	Test_Shadowsocks_NoMultiplex_NoPrefix = TestParams{
		Name: "shadowsocks-no-multiplex-no-prefix",
		ProxyConfig: &config.ProxyConfig{
			PluggableTransport: "shadowsocks",
			PluggableTransportSettings: map[string]string{
				"shadowsocks_secret": "secret",
				"shadowsocks_cipher": "chacha20-ietf-poly1305",
			},
			Addr:      DefaultTestAddr,
			AuthToken: "bunnyfoofoo",
			Cert:      string(util.MustReadFile(LocalHTTPProxyLanternTestCertFile)),
			Prefixes:  nil,
		},
		TestCases: testcases.DefaultTestCases,
	}
}
