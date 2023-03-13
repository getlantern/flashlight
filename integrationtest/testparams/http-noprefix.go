package testparams

import (
	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/testcases"
)

var Test_HTTP_NoPrefix TestParams

func init() {
	Test_HTTP_NoPrefix = TestParams{
		Name: "http-no-prefix",
		ProxyConfig: &config.ProxyConfig{
			PluggableTransport: "http",
			Addr:               DefaultTestAddr,
			AuthToken:          "bunnyfoofoo",
			Prefixes:           nil,
		},
		TestCases: testcases.DefaultTestCases,
	}
}
