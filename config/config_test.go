package config

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/golog"
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

		// No proxies on disk.
		assert.True(t, embeddedIsNewer(conf.filePath))

		conf.doSaveOne([]byte("test"))

		// Saved new proxies file -- make sure we use that.
		assert.False(t, embeddedIsNewer(conf.filePath))
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

func TestPollIntervals(t *testing.T) {
	withTempDir(t, func(inTempDir func(string) string) {

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

		fetcher := newHttpFetcher(newTestUserConfig(), http.DefaultClient, configURLs)
		dispatch := func(cfg interface{}) {}

		stopChan := make(chan bool)
		go cfg.configFetcher("testOpName", stopChan, dispatch, fetcher, func() time.Duration { return pollInterval }, log)
		time.Sleep(waitTime)
		close(stopChan)

		assert.Equal(t, 3, int(reqCount()), "should have fetched config every %v", pollInterval)
	})
}
