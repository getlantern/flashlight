package config

import (
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
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
	cfg := newConfig("./fetched-proxies.yaml", &options{
		unmarshaler: proxiesUnmarshaler,
	})

	pr, err := cfg.saved()
	assert.Nil(t, err)

	proxies := pr.(map[string]*chained.ChainedServerInfo)
	chained := proxies["fallback-104.236.192.114"]
	assert.True(t, chained != nil)
	assert.Equal(t, "104.236.192.114:443", chained.Addr)
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
	gzippedFile := "fetched-proxies.yaml.gz"
	cfg := newConfig(file, &options{
		unmarshaler: proxiesUnmarshaler,
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

	createGzippedFile(t, file, gzippedFile)

	mtime := fi.ModTime()
	oldName := fi.Name()
	tempName := oldName + ".stored"
	renameFile(t, oldName, tempName)
	defer renameFile(t, tempName, oldName)

	proxyConfigURLs := startConfigServer(t, gzippedFile)
	fetcher := newFetcher(&authConfig{}, &http.Transport{}, proxyConfigURLs)
	dispatch := func(cfg interface{}) {
		proxyChan <- cfg
	}
	go cfg.poll(dispatch, fetcher, func() time.Duration { return 1 * time.Hour })
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
}

func TestPollGlobal(t *testing.T) {
	fronted.ConfigureForTest(t)
	configChan := make(chan interface{})
	file := "./fetched-global.yaml"
	gzippedFile := "fetched-global.yaml.gz"
	cfg := newConfig(file, &options{
		unmarshaler: globalUnmarshaler,
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

	createGzippedFile(t, file, gzippedFile)

	mtime := fi.ModTime()
	oldName := fi.Name()
	tempName := oldName + ".stored"
	renameFile(t, oldName, tempName)
	defer renameFile(t, tempName, oldName)

	globalConfigURLs := startConfigServer(t, gzippedFile)
	fetcher := newFetcher(&authConfig{}, &http.Transport{}, globalConfigURLs)
	dispatch := func(cfg interface{}) {
		configChan <- cfg
	}
	go cfg.poll(dispatch, fetcher, func() time.Duration { return 1 * time.Hour })

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
}

func TestPollIntervals(t *testing.T) {
	fronted.ConfigureForTest(t)
	file := "./fetched-global.yaml"
	gzippedFile := "fetched-global.yaml.gz"
	cfg := newConfig(file, &options{
		unmarshaler: globalUnmarshaler,
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

	createGzippedFile(t, file, gzippedFile)

	oldName := fi.Name()
	tempName := oldName + ".stored"
	renameFile(t, oldName, tempName)
	defer renameFile(t, tempName, oldName)

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Unable to listen: %s", err)
	}

	var reqCount uint64
	hs := &http.Server{
		Handler: http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			atomic.AddUint64(&reqCount, 1)
			http.ServeFile(resp, req, gzippedFile)
		}),
	}
	go func() {
		if err = hs.Serve(l); err != nil {
			t.Fatalf("Unable to serve: %v", err)
		}
	}()

	port := l.Addr().(*net.TCPAddr).Port
	url := "http://localhost:" + strconv.Itoa(port)
	configURLs := &chainedFrontedURLs{
		chained: url,
		fronted: url,
	}
	pollInterval := 500 * time.Millisecond
	waitTime := pollInterval*2 + (200 * time.Millisecond)

	fetcher := newFetcher(&authConfig{}, &http.Transport{}, configURLs)
	dispatch := func(cfg interface{}) {}
	go cfg.poll(dispatch, fetcher, func() time.Duration { return pollInterval })

	time.Sleep(waitTime)

	finalReqCount := atomic.LoadUint64(&reqCount)
	assert.Equal(t, 3, int(finalReqCount), "should have fetched config every %v", pollInterval)
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

func startConfigServer(t *testing.T, configFilename string) (urls *chainedFrontedURLs) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Unable to listen: %s", err)
	}

	hs := &http.Server{
		Handler: http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			http.ServeFile(resp, req, configFilename)
		}),
	}
	go func() {
		if err = hs.Serve(l); err != nil {
			t.Fatalf("Unable to serve: %v", err)
		}
	}()

	port := l.Addr().(*net.TCPAddr).Port
	url := "http://localhost:" + strconv.Itoa(port)
	return &chainedFrontedURLs{
		chained: url,
		fronted: url,
	}
}

func renameFile(t *testing.T, oldpath string, newpath string) {
	err := os.Rename(oldpath, newpath)
	if err != nil {
		t.Fatalf("Unable to rename file: %s", err)
	}
}
