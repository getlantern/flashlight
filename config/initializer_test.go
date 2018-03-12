package config

import (
	"net/http"
	"testing"
	"time"

	"github.com/getlantern/flashlight/chained"
	"github.com/stretchr/testify/assert"
)

// TestInit tests initializing configs.
func TestInit(t *testing.T) {
	defer deleteGlobalConfig()

	flags := make(map[string]interface{})
	flags["staging"] = true

	configChan := make(chan bool)

	// Note these dispatch functions will receive multiple configs -- local ones,
	// embedded ones, and remote ones.
	proxiesDispatch := func(cfg interface{}) {
		proxies := cfg.(map[string]*chained.ChainedServerInfo)
		assert.True(t, len(proxies) > 0)
		configChan <- true
	}
	globalDispatch := func(cfg interface{}) {
		global := cfg.(*Global)
		assert.True(t, len(global.Client.MasqueradeSets) > 1)
		configChan <- true
	}
	stop := Init(".", flags, &authConfig{}, proxiesDispatch, globalDispatch, &http.Transport{})
	defer stop()

	count := 0
	for i := 0; i < 2; i++ {
		select {
		case <-configChan:
			count++
		case <-time.After(time.Second * 12):
			assert.Fail(t, "Took too long to get configs")
		}
	}
	assert.Equal(t, 2, count)
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
		globalConfigURLs, globalReqCount := startConfigServer(t, globalConfig)

		// set up servers to serve global config and count number of requests
		proxyConfigURLs, proxyReqCount := startConfigServer(t, proxiesConfig)

		// set up and call InitWithURLs
		flags := make(map[string]interface{})
		flags["staging"] = true

		// Note these dispatch functions will receive multiple configs -- local ones,
		// embedded ones, and remote ones.
		proxiesDispatch := func(cfg interface{}) {}
		globalDispatch := func(cfg interface{}) {}
		stop := InitWithURLs(inTempDir("."), flags, &authConfig{}, proxiesDispatch, globalDispatch,
			proxyConfigURLs, globalConfigURLs, &http.Transport{})
		defer stop()

		// sleep some amount
		time.Sleep(6500 * time.Millisecond)
		// in 6.5 sec, should have made:
		// - 1 + (6 / 3) = 3 global requests
		// - 1 + (6 / 1) = 7 proxy requests

		// test that proxy & config servers were called the correct number of times
		assert.Equal(t, 3, int(globalReqCount()), "should have fetched global config every %v", globalConfig.GlobalConfigPollInterval)
		assert.Equal(t, 7, int(proxyReqCount()), "should have fetched proxy config every %v", globalConfig.ProxyConfigPollInterval)
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
	urls := &chainedFrontedURLs{
		chained: "chained",
		fronted: "fronted",
	}
	flags := make(map[string]interface{})
	checkOverrides(flags, urls, "name")

	assert.Equal(t, "chained", urls.chained)
	assert.Equal(t, "fronted", urls.fronted)

	flags["cloudconfig"] = "test"
	checkOverrides(flags, urls, "name")

	assert.Equal(t, "test/name", urls.chained)
	assert.Equal(t, "fronted", urls.fronted)

	flags["frontedconfig"] = "test"
	checkOverrides(flags, urls, "name")

	assert.Equal(t, "test/name", urls.chained)
	assert.Equal(t, "test/name", urls.fronted)
}
