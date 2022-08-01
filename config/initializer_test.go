package config

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/getlantern/eventual"
	"github.com/getlantern/common/apipb"
	"github.com/getlantern/flashlight/common"
	"github.com/stretchr/testify/assert"
)

// TestInit tests initializing configs.
func TestInit(t *testing.T) {
	defer deleteGlobalConfig()

	flags := make(map[string]interface{})
	flags["staging"] = true

	gotProxies := eventual.NewValue()
	gotGlobal := eventual.NewValue()

	// Note these dispatch functions will receive multiple configs -- local ones,
	// embedded ones, and remote ones.
	proxiesDispatch := func(cfg interface{}, src Source) {
		proxies := cfg.(map[string]*apipb.ProxyConfig)
		assert.True(t, len(proxies) > 0)
		gotProxies.Set(true)
	}
	globalDispatch := func(cfg interface{}, src Source) {
		global := cfg.(*Global)
		assert.True(t, len(global.Client.MasqueradeSets) > 1)
		gotGlobal.Set(true)
	}
	stop := Init(
		".", flags, newTestUserConfig(), proxiesDispatch, nil, globalDispatch, nil, &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				// the same token should also be configured on staging
				// config-server, staging proxies and staging DDF distributions.
				req.Header.Add(common.CfgSvrAuthTokenHeader, "staging-token")
				return nil, nil
			},
		}, nil)
	defer stop()

	_, valid := gotProxies.Get(time.Second * 12)
	assert.True(t, valid, "Should have got proxies config in a reasonable time")
	_, valid = gotGlobal.Get(time.Second * 12)
	assert.True(t, valid, "Should have got global config in a reasonable time")
}

// TestInitWithURLs tests that proxy and global configs are fetched at the
// correct polling intervals.
func TestInitWithURLs(t *testing.T) {
	withTempDir(t, func(inTempDir func(string) string) {
		globalConfig := newGlobalConfig(t)
		proxiesConfig := newProxiesConfig(t)

		globalConfig.GlobalConfigPollInterval = 3 * time.Second
		globalConfig.ProxyConfigPollInterval = 1 * time.Second

		// ensure a `global.yaml` exists in order to avoid fetching embedded config
		writeObfuscatedConfig(t, globalConfig, inTempDir("global.yaml"))

		// set up 2 servers:
		// 1. one that serves up the global config and
		// 2. one that serves up the proxy config
		// each should track the number of requests made to it

		// set up servers to serve global config and count number of requests
		globalConfigURL, globalReqCount := startConfigServer(t, globalConfig)

		// set up servers to serve global config and count number of requests
		proxyConfigURL, proxyReqCount := startConfigServer(t, proxiesConfig)

		// set up and call InitWithURLs
		flags := make(map[string]interface{})
		flags["staging"] = true

		proxiesDispatch := func(interface{}, Source) {}
		globalDispatch := func(interface{}, Source) {}
		stop := InitWithURLs(
			inTempDir("."), flags, newTestUserConfig(),
			proxiesDispatch, nil,
			globalDispatch, nil,
			proxyConfigURL, globalConfigURL, &http.Transport{}, nil)
		defer stop()

		// sleep some amount
		time.Sleep(7 * time.Second)
		// in 7 sec, should have made:
		//  1 + (7 / 3) = 3 global requests
		//  1 + (7 / 1) = 8 proxy requests
		// We provide a little leeway in the checks below to account for possible delays in CI.

		// test that proxy & config servers were called the correct number of times
		assert.GreaterOrEqual(t, 3, int(globalReqCount()), "should have fetched global config every %v", globalConfig.GlobalConfigPollInterval)
		assert.GreaterOrEqual(t, 7, int(proxyReqCount()), "should have fetched proxy config every %v", globalConfig.ProxyConfigPollInterval)
	})
}

func TestStaging(t *testing.T) {
	flags := make(map[string]interface{})
	flags["staging"] = true

	assert.True(t, isStaging(flags))

	flags["staging"] = false

	assert.False(t, isStaging(flags))
}

// TestOverrides tests url override flags
func TestOverrides(t *testing.T) {
	url := "host"
	flags := make(map[string]interface{})
	out := checkOverrides(flags, url, "name")

	assert.Equal(t, "host", out)

	flags["cloudconfig"] = "test"
	out = checkOverrides(flags, url, "name")

	assert.Equal(t, "test/name", out)
}
