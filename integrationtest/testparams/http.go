package testparams

import (
	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/testcases"
)

var Test_HTTP TestParams

func init() {
	Test_HTTP = TestParams{
		Name: "http",
		ProxyConfig: &config.ProxyConfig{
			PluggableTransport: "http",
			Addr:               DefaultTestAddr,
			AuthToken:          "bunnyfoofoo",
		},
		TestCases: testcases.DefaultTestCases,
	}
}
