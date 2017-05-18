package config

import (
	"errors"
	"io/ioutil"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/config/generated"
	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/fronted"
)

// TestInvalidFile test an empty or malformed config file
func TestInvalidFile(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "invalid-test-file")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name()) // clean up

	configPath := tmpfile.Name()

	t.Log("path: %v", configPath)
	c := &config{}
	c.Reconfigure(nil, &ConfigOpts{})
	_, err = c.saved(&FetchOpts{fullPath: configPath})
	assert.Error(t, err, "should get error if config file is empty")

	tmpfile.WriteString("content: anything")
	tmpfile.Sync()
	var expectedError = errors.New("invalid content")
	_, err = c.saved(&FetchOpts{fullPath: configPath, unmarshaler: func([]byte) (service.Message, error) {
		return nil, expectedError
	}})
	assert.Equal(t, expectedError, err,
		"should get application specific unmarshal error")
}

// TestObfuscated tests reading obfuscated global config from disk
func TestObfuscated(t *testing.T) {
	c := &config{}
	c.Reconfigure(nil, &ConfigOpts{
		Obfuscate: true,
	})
	conf, err := c.saved(&FetchOpts{
		fullPath:    "./obfuscated-global.yaml",
		unmarshaler: c.unmarshalGlobal,
	})
	assert.Nil(t, err)

	cfg := conf.(*Global)

	// Just make sure it's legitimately reading the config.
	assert.True(t, len(cfg.Client.MasqueradeSets) > 1)
}

func TestOverrideGlobal(t *testing.T) {
	c := &config{}
	c.Reconfigure(nil, &ConfigOpts{
		Obfuscate: true,
		OverrideGlobal: func(gl *Global) {
			gl.AutoUpdateCA = "overriden"
		},
	})
	conf, err := c.saved(&FetchOpts{
		fullPath:    "./obfuscated-global.yaml",
		unmarshaler: c.unmarshalGlobal,
	})
	assert.Nil(t, err)
	assert.Equal(t, "overriden", conf.(*Global).AutoUpdateCA)
}

// TestSaved tests reading stored proxies from disk
func TestSaved(t *testing.T) {
	c := &config{}
	c.Reconfigure(nil, &ConfigOpts{})
	pr, err := c.saved(&FetchOpts{
		fullPath:    "./test-proxies.yaml",
		unmarshaler: c.unmarshalProxies,
	})
	assert.Nil(t, err)

	proxies := pr.(Proxies)
	chained := proxies["fallback-1.1.1.1"]
	assert.True(t, chained != nil)
	assert.Equal(t, "1.1.1.1:443", chained.Addr)
}

// TestEmbedded tests reading stored proxies from disk
func TestEmbedded(t *testing.T) {
	c := &config{}
	c.Reconfigure(nil, &ConfigOpts{Obfuscate: true})
	pr, err := c.embedded(&FetchOpts{
		EmbeddedName: "proxies.yaml",
		EmbeddedData: generated.EmbeddedProxies,
		unmarshaler:  c.unmarshalProxies,
	})
	assert.Nil(t, err)

	proxies := pr.(Proxies)
	assert.Equal(t, 6, len(proxies))
	for _, val := range proxies {
		assert.True(t, val != nil)
		assert.True(t, len(val.Addr) > 6)
	}
}

type publisher struct {
	wg      sync.WaitGroup
	proxies Proxies
	global  *Global
}

func (p *publisher) Publish(m service.Message) {
	switch actual := m.(type) {
	case Proxies:
		p.proxies = actual
		p.wg.Done()
	case *Global:
		p.global = actual
		p.wg.Done()
	default:
		panic("should never happen")
	}
}

func TestPoll(t *testing.T) {
	fronted.ConfigureForTest(t)
	p := &publisher{}
	// the service publishes initial config first, then polling from server,
	// that's why we set wg to 4
	p.wg.Add(4)
	proxiesFile := "./fetched-proxies.yaml"
	globalFile := "./fetched-global.yaml"
	opts := DefaultConfigOpts()
	opts.Proxies.FileName = proxiesFile
	opts.Global.FileName = globalFile
	c := New()
	c.Reconfigure(p, opts)

	start := time.Now()
	renameBack := renameFile(t, proxiesFile)
	defer renameBack()
	renameBack = renameFile(t, globalFile)
	defer renameBack()

	c.Start()
	p.wg.Wait()

	assert.True(t, len(p.proxies) > 0, "should have fetched proxies")
	for _, val := range p.proxies {
		assert.NotNil(t, val, "should have fetched proxies in correct format")
		assert.True(t, len(val.Addr) > 6)
	}

	assert.True(t, len(p.global.Client.MasqueradeSets) > 1, "global config should have masquerade sets")

	assert.True(t, waitUntilModified(proxiesFile, start), "should update file on disk")
	assert.True(t, waitUntilModified(globalFile, start), "should update file on disk")

	fi, err := os.Stat(proxiesFile)
	assert.NoError(t, err)
	assert.NotNil(t, fi)

	fi, err = os.Stat(globalFile)
	assert.NoError(t, err)
	assert.NotNil(t, fi)
}

func waitUntilModified(file string, t time.Time) bool {
	for i := 1; i <= 400; i++ {
		fi, err := os.Stat(file)
		if err == nil && fi != nil && fi.ModTime().After(t) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func renameFile(t *testing.T, file string) func() {
	fi, err := os.Stat(file)
	if !assert.NoError(t, err) {
		return func() {}
	}
	tempName := fi.Name() + ".stored"
	os.Rename(fi.Name(), tempName)
	return func() { os.Rename(tempName, fi.Name()) }
}
