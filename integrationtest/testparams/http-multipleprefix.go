package testparams

import (
	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/testcases"
)

var Test_HTTP_MultiplePrefix TestParams

func init() {
	Test_HTTP_MultiplePrefix = TestParams{
		Name: "http-multiple-prefix",
		ProxyConfig: &config.ProxyConfig{
			PluggableTransport: "http",
			Addr:               DefaultTestAddr,
			AuthToken:          "bunnyfoofoo",
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
