package tests

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/rediswrapper"
	"github.com/getlantern/flashlight-integration-test/util"
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

func initHttpProxyLanternLocalOrRemote(
	rdb *redis.Client,
	integrationTestConfig *IntegrationTestConfig,
	remoteTestTrackName string,
	localProxyConfig *config.ProxyConfig,
) (*config.ProxyConfig, io.Closer, error) {
	var proxyConfig *config.ProxyConfig
	var httpProxyLanternHandle io.Closer
	var err error
	if integrationTestConfig.IsHttpProxyLanternLocal {
		httpProxyLanternHandle, err = initLocalHttpProxyLantern(localProxyConfig)
		if err != nil {
			return nil, nil,
				fmt.Errorf("Unable to init local http-proxy-lantern: %v", err)
		}
		defer httpProxyLanternHandle.Close()
		proxyConfig = localProxyConfig
	} else {
		ctx, cancel := context.WithTimeout(
			context.Background(), 5*time.Second)
		defer cancel()
		proxyConfig, err = rediswrapper.FetchRandomProxyConfigFromTrack(
			ctx, rdb, remoteTestTrackName)
		if err != nil {
			return nil, nil,
				fmt.Errorf(
					"Unable to fetch random proxy from track %s: %v",
					remoteTestTrackName, err)
		}
		httpProxyLanternHandle = util.IoNopCloser{}
	}

	return proxyConfig, httpProxyLanternHandle, nil
}

func initLocalHttpProxyLantern(proxyConfig *config.ProxyConfig) (*httpProxyLantern.Proxy, error) {
	return nil, nil
}
