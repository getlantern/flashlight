package testparams

import (
	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/testcases"
	"github.com/getlantern/flashlight-integration-test/util"
)

var Test_HTTPS TestParams

func init() {
	Test_HTTPS = TestParams{
		Name: "https",
		ProxyConfig: &config.ProxyConfig{
			PluggableTransport: "https",
			Addr:               DefaultTestAddr,
			AuthToken:          "bunnyfoofoo",
			Cert:               string(util.MustReadFile(LocalHTTPProxyLanternTestCertFile)),
		},
		TestCases: testcases.DefaultTestCases,
	}
}
