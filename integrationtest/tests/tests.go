package tests

import (
	"fmt"
	"io"

	"github.com/getlantern/common/config"
	httpProxyLantern "github.com/getlantern/http-proxy-lantern/v2"
	"github.com/go-redis/redis/v8"
)

var allTests map[string]Test

func init() {
	allTests = map[string]Test{
		// Shadowsocks
		// ---------------
		// "shadowsocks-nomultiplex-noprefix": &Test_Shadowsocks_NoMultiplex_NoPrefix{},
		// "shadowsocks-nomultiplex-singleprefix": &Test_Shadowsocks_NoMultiplex_SinglePrefix{},
		"shadowsocks-nomultiplex-multipleprefix": &Test_Shadowsocks_NoMultiplex_MultiplePrefix{},
	}
}

func Run(rdb *redis.Client, test string, integrationTestConfig *IntegrationTestConfig) error {
	if test == "all" {
		return testAll(rdb, integrationTestConfig)
	}

	t, ok := allTests[test]
	if !ok {
		return fmt.Errorf("Test %s not found", test)
	}

	proxyConfig, httpProxyLanternHandle, err := t.Init(rdb, integrationTestConfig)
	if err != nil {
		return fmt.Errorf("Unable to init test %s: %s", test, err)
	}
	defer httpProxyLanternHandle.Close()
	return t.Run(proxyConfig)
}

func testAll(rdb *redis.Client, integrationTestConfig *IntegrationTestConfig) error {
	for name, test := range allTests {
		proxyConfig, httpProxyLanternHandle, err := test.Init(rdb, integrationTestConfig)
		if err != nil {
			return fmt.Errorf("Unable to init test %s: %s", test, err)
		}
		defer httpProxyLanternHandle.Close()

		if err := test.Run(proxyConfig); err != nil {
			return fmt.Errorf("Test %s failed: %s", name, err)
		}
	}

	return nil
}

type TestCase struct {
	connectionType           string
	testURL                  string
	expectedStringInResponse string
}

type TestCaseAndError struct {
	TestCase
	err error
}

type IntegrationTestConfig struct {
	IsHttpProxyLanternLocal bool
}

type Test interface {
	Init(*redis.Client, *IntegrationTestConfig) (
		proxyConfig *config.ProxyConfig,
		httpProxyLanternHandle io.Closer,
		err error)
	Run(*config.ProxyConfig) error
}

func initLocalHttpProxyLantern(proxyConfig *config.ProxyConfig) (*httpProxyLantern.Proxy, error) {
	return nil, nil
}
