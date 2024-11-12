package config

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/fronted"
	"github.com/getlantern/golog"

	"github.com/getlantern/flashlight/v7/common"
)

func TestEmptyEmbedded(t *testing.T) {
	withTempDir(t, func(inTempDir func(string) string) {
		opts := &options{
			name: "test",
		}
		configPath := inTempDir(opts.name)
		conf := newConfig(configPath, opts)

		_, err := conf.embedded([]byte(``))
		assert.Error(t, err, "should get error if embedded config is empty")
	})
}

func TestEmbeddedIsNewer(t *testing.T) {
	withTempDir(t, func(inTempDir func(string) string) {
		opts := &options{
			name: "test",
		}
		configPath := inTempDir(opts.name)
		conf := newConfig(configPath, opts)

		// No embedded config.
		assert.False(t, embeddedIsNewer(conf, opts))

		opts.embeddedData = []byte("test")

		// No proxies on disk.
		assert.True(t, embeddedIsNewer(conf, opts))

		conf.doSaveOne(opts.embeddedData)

		// Saved new proxies file -- make sure we use that.
		assert.False(t, embeddedIsNewer(conf, opts))
	})
}

// TestInvalidFile test an empty or malformed config file
func TestInvalidFile(t *testing.T) {
	logger := golog.LoggerFor("config-test")

	tmpfile, err := os.CreateTemp("", "invalid-test-file")
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

// TestProductionGlobal validates certain properties of the live production global config
func TestProductionGlobal(t *testing.T) {
	testURL := common.GlobalURL // this should always point to the live production configuration (not staging etc)

	expectedProviders := map[string]bool{
		"akamai":     true,
		"cloudfront": true,
	}

	f := newHttpFetcher(newTestUserConfig(), &http.Transport{}, testURL)

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

		fetcher := newHttpFetcher(newTestUserConfig(), &http.Transport{}, configURLs)
		dispatch := func(cfg interface{}) {}

		stopChan := make(chan bool)
		go cfg.configFetcher(stopChan, dispatch, fetcher, func() time.Duration { return pollInterval }, log)
		time.Sleep(waitTime)
		close(stopChan)

		assert.Equal(t, 3, int(reqCount()), "should have fetched config every %v", pollInterval)
	})
}
