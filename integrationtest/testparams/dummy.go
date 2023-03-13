package testparams

import (
	"github.com/getlantern/common/config"
)

var Test_Dummy TestParams

func init() {
	Test_Dummy = TestParams{
		Name: "dummy",
		ProxyConfig: &config.ProxyConfig{
			Addr: DefaultTestAddr,
		},
		TestCases: nil,
	}
}
