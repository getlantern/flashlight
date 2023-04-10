package testparams

import (
	"path/filepath"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/projectpath"
	"github.com/getlantern/flashlight-integration-test/testcases"
)

var LocalHTTPProxyLanternTestKeyFile = filepath.Join(
	projectpath.Root,
	"testdata",
	"testkey.pem")
var LocalHTTPProxyLanternTestCertFile = filepath.Join(
	projectpath.Root,
	"testdata",
	"testcert.pem")
var DefaultTestAddr = "localhost:41234"

type TestParams struct {
	Name        string
	ProxyConfig *config.ProxyConfig
	TestCases   []testcases.TestCase
}
