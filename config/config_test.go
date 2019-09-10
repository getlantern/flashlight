package config

import (
	"errors"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/getlantern/fronted"
	"github.com/getlantern/golog"
	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/config/generated"
)

// TestInvalidFile test an empty or malformed config file
func TestInvalidFile(t *testing.T) {
	logger := golog.LoggerFor("config-test")

	tmpfile, err := ioutil.TempFile("", "invalid-test-file")
	if err != nil {
		logger.Fatal(err)
	}
	defer os.Remove(tmpfile.Name()) // clean up

	configPath := tmpfile.Name()

	logger.Debugf("path: %v", configPath)
	conf := newConfig(configPath, &options{})
	_, proxyErr := conf.saved()
	assert.Error(t, proxyErr, "should get error if config file is empty")

	tmpfile.WriteString("content: anything")
	tmpfile.Sync()
	var expectedError = errors.New("invalid content")
	conf = newConfig(configPath, &options{
		unmarshaler: func([]byte) (interface{}, error) {
			return nil, expectedError
		},
	})
	_, proxyErr = conf.saved()
	assert.Equal(t, expectedError, proxyErr,
		"should get application specific unmarshal error")
}

// TestObfuscated tests reading obfuscated global config from disk
func TestObfuscated(t *testing.T) {
	withTempDir(t, func(inTempDir func(string) string) {
		file := inTempDir("global.yaml")
		globalConfig := newGlobalConfig(t)
		writeObfuscatedConfig(t, globalConfig, file)

		config := newConfig(file, &options{
			obfuscate:   true,
			unmarshaler: newGlobalUnmarshaler(nil),
		})

		conf, err := config.saved()
		assert.Nil(t, err)

		cfg := conf.(*Global)

		// Just make sure it's legitimately reading the config.
		assert.True(t, len(cfg.Client.MasqueradeSets) > 1)
	})
}

// TestSaved tests reading stored proxies from disk
func TestSaved(t *testing.T) {
	withTempDir(t, func(inTempDir func(string) string) {
		file := inTempDir("proxies.yaml")
		proxiesConfig := newProxiesConfig(t)
		writeObfuscatedConfig(t, proxiesConfig, file)

		cfg := newConfig(file, &options{
			obfuscate:   true,
			unmarshaler: newProxiesUnmarshaler(),
		})

		pr, err := cfg.saved()
		assert.Nil(t, err)

		proxies := pr.(map[string]*chained.ChainedServerInfo)
		chained := proxies["fallback-104.236.192.114"]
		assert.True(t, chained != nil)
		assert.Equal(t, "104.236.192.114:443", chained.Addr)
	})
}

// TestEmbedded tests reading stored proxies from disk
func TestEmbedded(t *testing.T) {
	withTempDir(t, func(inTempDir func(string) string) {
		file := inTempDir("proxies.yaml")

		cfg := newConfig(file, &options{
			unmarshaler: newProxiesUnmarshaler(),
		})

		proxies, _ := cfg.embedded(generated.EmbeddedProxies)
		assert.NotNil(t, proxies)
	})
}

func TestPollProxies(t *testing.T) {
	withTempDir(t, func(inTempDir func(string) string) {
		fronted.ConfigureForTest(t)

		file := inTempDir("proxies.yaml")
		proxyConfig := newProxiesConfig(t)
		writeObfuscatedConfig(t, proxyConfig, file)

		proxyChan := make(chan interface{})
		cfg := newConfig(file, &options{
			unmarshaler: newProxiesUnmarshaler(),
		})
		var fi os.FileInfo
		var err error
		for i := 1; i <= 400; i++ {
			fi, err = os.Stat(file)
			if err == nil {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if !assert.Nil(t, err) {
			return
		}

		mtime := fi.ModTime()
		os.Remove(file)

		proxyConfigURLs, _ := startConfigServer(t, proxyConfig)
		fetcher := newFetcher(newTestUserConfig(), &http.Transport{}, proxyConfigURLs)
		dispatch := func(cfg interface{}) {
			proxyChan <- cfg
		}
		go cfg.poll(nil, dispatch, fetcher, func() time.Duration { return 1 * time.Hour })
		proxies := (<-proxyChan).(map[string]*chained.ChainedServerInfo)

		assert.True(t, len(proxies) > 0)
		for _, val := range proxies {
			assert.True(t, val != nil)
			assert.True(t, len(val.Addr) > 6)
		}

		for i := 1; i <= 400; i++ {
			fi, err = os.Stat(file)
			if err == nil && fi != nil && fi.ModTime().After(mtime) {
				//log.Debugf("Got newer mod time?")
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		fi, err = os.Stat(file)

		assert.NotNil(t, fi)
		assert.Nil(t, err, "Got error: %v", err)

		assert.True(t, fi.ModTime().After(mtime))
	})
}

func TestPollGlobal(t *testing.T) {
	withTempDir(t, func(inTempDir func(string) string) {
		fronted.ConfigureForTest(t)

		file := inTempDir("global.yaml")
		globalConfig := newGlobalConfig(t)
		writeObfuscatedConfig(t, globalConfig, file)

		configChan := make(chan interface{})
		cfg := newConfig(file, &options{
			unmarshaler: newGlobalUnmarshaler(nil),
		})
		var fi os.FileInfo
		var err error
		for i := 1; i <= 400; i++ {
			fi, err = os.Stat(file)
			if err == nil {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if !assert.Nil(t, err) {
			return
		}

		mtime := fi.ModTime()
		os.Remove(file)

		globalConfigURLs, _ := startConfigServer(t, globalConfig)
		fetcher := newFetcher(newTestUserConfig(), &http.Transport{}, globalConfigURLs)
		dispatch := func(cfg interface{}) {
			configChan <- cfg
		}
		go cfg.poll(nil, dispatch, fetcher, func() time.Duration { return 1 * time.Hour })

		var fetched *Global
		select {
		case fetchedConfig := <-configChan:
			fetched = fetchedConfig.(*Global)
		case <-time.After(20 * time.Second):
			break
		}

		if assert.False(t, fetched == nil, "fetching global config should have succeeded") {
			t.Log(fetched)
			assert.True(t, len(fetched.Client.MasqueradeSets) > 1, "global config should have masquerade sets")
		}

		for i := 1; i <= 400; i++ {
			fi, err = os.Stat(file)
			if err == nil && fi != nil && fi.ModTime().After(mtime) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		fi, err = os.Stat(file)
		if assert.Nil(t, err, "Got error: %v", err) {
			assert.NotNil(t, fi)
			assert.True(t, fi.ModTime().After(mtime), "Incorrect modification times")
		}
	})
}

// TestProductionGlobal validates certain properties of the live production global config
func TestProductionGlobal(t *testing.T) {

	testURL := globalURL // this should always point to the live production configuration (not staging etc)

	expectedProviders := map[string]bool{
		"cloudfront": true,
		"akamai":     true,
	}

	f := newFetcher(newTestUserConfig(), &http.Transport{}, testURL)

	cfgBytes, _, err := f.fetch()
	if !assert.NoError(t, err, "Error fetching global config from %s", testURL) {
		return
	}

	unmarshal := newGlobalUnmarshaler(nil)
	cfgIf, err := unmarshal(cfgBytes)
	if !assert.NoError(t, err, "Error unmarshaling global config from %s", testURL) {
		return
	}

	cfg, ok := cfgIf.(*Global)
	if !assert.True(t, ok, "Unexpected configuration type returned from %s", testURL) {
		return
	}

	defaultMasq := cfg.Client.MasqueradeSets["cloudfront"]
	assert.True(t, len(defaultMasq) > 500, "global config %s should have a large number of default masquerade sets for cloudfront (found %d)", testURL, len(defaultMasq))

	if !assert.NotNil(t, cfg.Client.Fronted, "global config %s missing fronted section!", testURL) {
		return
	}

	for pid := range expectedProviders {
		provider := cfg.Client.Fronted.Providers[pid]
		if !assert.NotNil(t, provider, "global config %s missing expected fronted provider %s", testURL, pid) {
			continue
		}
		assert.True(t, len(provider.Masquerades) > 100, "global config %s provider %s had only %d masquerades!", testURL, pid, len(provider.Masquerades))
		assert.True(t, len(provider.HostAliases) > 0, "global config %s provider %s has no host aliases?", testURL, pid)

	}

	for pid := range cfg.Client.Fronted.Providers {
		assert.True(t, expectedProviders[pid], "global config %s had unexpected provider %s (update expected list?)", testURL, pid)
	}
}

func TestPollIntervals(t *testing.T) {
	withTempDir(t, func(inTempDir func(string) string) {
		fronted.ConfigureForTest(t)

		file := inTempDir("global.yaml")
		globalConfig := newGlobalConfig(t)
		writeObfuscatedConfig(t, globalConfig, file)

		cfg := newConfig(file, &options{
			unmarshaler: newGlobalUnmarshaler(nil),
		})
		var err error
		for i := 1; i <= 400; i++ {
			_, err = os.Stat(file)
			if err == nil {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if !assert.Nil(t, err) {
			return
		}

		os.Remove(file)

		configURLs, reqCount := startConfigServer(t, globalConfig)
		pollInterval := 500 * time.Millisecond
		waitTime := pollInterval*2 + (200 * time.Millisecond)

		fetcher := newFetcher(newTestUserConfig(), &http.Transport{}, configURLs)
		dispatch := func(cfg interface{}) {}

		stopChan := make(chan bool)
		go cfg.poll(stopChan, dispatch, fetcher, func() time.Duration { return pollInterval })
		time.Sleep(waitTime)
		close(stopChan)

		assert.Equal(t, 3, int(reqCount()), "should have fetched config every %v", pollInterval)
	})
}
