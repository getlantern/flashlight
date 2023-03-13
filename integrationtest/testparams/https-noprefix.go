package testparams

import (
	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/testcases"
	"github.com/getlantern/flashlight-integration-test/util"
)

var Test_HTTPS_NoPrefix TestParams

func init() {
	Test_HTTPS_NoPrefix = TestParams{
		Name: "https-no-prefix",
		ProxyConfig: &config.ProxyConfig{
			PluggableTransport: "https",
			Addr:               DefaultTestAddr,
			AuthToken:          "bunnyfoofoo",
			Cert:               string(util.MustReadFile(LocalHTTPProxyLanternTestCertFile)),
			Prefixes:           nil,
		},
		TestCases: testcases.DefaultTestCases,
	}
}
