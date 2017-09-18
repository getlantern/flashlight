package config

import (
	"errors"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/getlantern/fronted"
	"github.com/getlantern/golog"
	"github.com/getlantern/yaml"
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
	config := newConfig("./obfuscated-global.yaml", &options{
		obfuscate:   true,
		unmarshaler: globalUnmarshaler,
	})

	conf, err := config.saved()
	assert.Nil(t, err)

	cfg := conf.(*Global)

	// Just make sure it's legitimately reading the config.
	assert.True(t, len(cfg.Client.MasqueradeSets) > 1)
}

// TestSaved tests reading stored proxies from disk
func TestSaved(t *testing.T) {
	cfg := newConfig("./test-proxies.yaml", &options{
		unmarshaler: proxiesUnmarshaler,
	})

	pr, err := cfg.saved()
	assert.Nil(t, err)

	proxies := pr.(map[string]*chained.ChainedServerInfo)
	chained := proxies["fallback-1.1.1.1"]
	assert.True(t, chained != nil)
	assert.Equal(t, "1.1.1.1:443", chained.Addr)
}

// TestEmbedded tests reading stored proxies from disk
func TestEmbedded(t *testing.T) {
	cfg := newConfig("./test-proxies.yaml", &options{
		unmarshaler: proxiesUnmarshaler,
	})

	pr, err := cfg.embedded(generated.EmbeddedProxies, "proxies.yaml")
	assert.Nil(t, err)

	proxies := pr.(map[string]*chained.ChainedServerInfo)
	assert.Equal(t, 6, len(proxies))
	for _, val := range proxies {
		assert.True(t, val != nil)
		assert.True(t, len(val.Addr) > 6)
	}
}

func TestPollProxies(t *testing.T) {
	fronted.ConfigureForTest(t)
	proxyChan := make(chan interface{})
	file := "./fetched-proxies.yaml"
	cfg := newConfig(file, &options{
		unmarshaler: proxiesUnmarshaler,
	})

	fi, err := os.Stat(file)
	if !assert.Nil(t, err) {
		return
	}
	mtime := fi.ModTime()
	tempName := fi.Name() + ".stored"
	os.Rename(fi.Name(), tempName)

	urls := proxiesURLs
	go cfg.poll(&authConfig{}, proxyChan, urls, 1*time.Hour)
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

	// Just restore the original file.
	os.Rename(tempName, fi.Name())
}

func TestPollGlobal(t *testing.T) {
	fronted.ConfigureForTest(t)
	configChan := make(chan interface{})
	file := "./fetched-global.yaml"
	cfg := newConfig(file, &options{
		unmarshaler: globalUnmarshaler,
	})

	fi, err := os.Stat(file)
	if !assert.Nil(t, err) {
		return
	}
	mtime := fi.ModTime()
	tempName := fi.Name() + ".stored"
	os.Rename(fi.Name(), tempName)

	go cfg.poll(&authConfig{}, configChan, globalURLs, 1*time.Hour)

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

	// Just restore the original file.
	os.Rename(tempName, file)
}

func globalUnmarshaler(b []byte) (interface{}, error) {
	gl := &Global{}
	err := yaml.Unmarshal(b, gl)
	return gl, err
}

func proxiesUnmarshaler(b []byte) (interface{}, error) {
	servers := make(map[string]*chained.ChainedServerInfo)
	err := yaml.Unmarshal(b, servers)
	return servers, err
}
