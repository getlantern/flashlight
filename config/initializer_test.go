package config

import (
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlantern/flashlight/chained"
	"github.com/stretchr/testify/assert"
)

// TestInit tests initializing configs.
func TestInit(t *testing.T) {
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
	Init(".", flags, &authConfig{}, proxiesDispatch, globalDispatch, &http.Transport{})

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
	// ensure a `global.yaml` exists in order to avoid fetching embedded config
	from, err := os.Open("./embedded-global.yaml")
	if err != nil {
		t.Fatalf("Unable to open file ./embedded-global.yaml: %s", err)
	}
	defer from.Close()

	to, err := os.OpenFile("./global.yaml", os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		t.Fatalf("Unable to open file global.yaml: %s", err)
	}
	defer to.Close()

	_, err = io.Copy(to, from)
  if err != nil {
		t.Fatalf("Unable to copy file: %s", err)
  }

	// set up 2 servers:
	// 1. one that serves up the global config and
	// 2. one that serves up the proxy config
	// each should track the number of requests made to it

	// set up listeners for global & proxy config servers
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Unable to listen: %s", err)
	}
	l2, err2 := net.Listen("tcp", "localhost:0")
	if err2 != nil {
		t.Fatalf("Unable to listen: %s", err2)
	}

	// set up servers to serve global config and count number of requests
	var globalReqCount uint64
	globalHs := &http.Server{
		Handler: http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			atomic.AddUint64(&globalReqCount, 1)
			http.ServeFile(resp, req, "./fetched-global.yaml.gz")
		}),
	}
	go func() {
		if err = globalHs.Serve(l); err != nil {
			t.Fatalf("Unable to serve: %v", err)
		}
	}()

	// set up servers to serve global config and count number of requests
	var proxyReqCount uint64
	proxyHs := &http.Server{
		Handler: http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			atomic.AddUint64(&proxyReqCount, 1)
			http.ServeFile(resp, req, "./fetched-proxies.yaml.gz")
		}),
	}
	go func() {
		if err2 = proxyHs.Serve(l2); err2 != nil {
			t.Fatalf("Unable to serve: %v", err2)
		}
	}()

	// set URL to fetch global config from server
	globalPort := l.Addr().(*net.TCPAddr).Port
	globalURL := "http://localhost:" + strconv.Itoa(globalPort)
	globalConfigURLs := &chainedFrontedURLs{
		chained: globalURL,
		fronted: globalURL,
	}

	// set URL to fetch proxy config from server
	proxyPort := l2.Addr().(*net.TCPAddr).Port
	proxyURL := "http://localhost:" + strconv.Itoa(proxyPort)
	proxyConfigURLs := &chainedFrontedURLs{
		chained: proxyURL,
		fronted: proxyURL,
	}

	// set up and call InitWithURLs
	flags := make(map[string]interface{})
	flags["staging"] = true

	// Note these dispatch functions will receive multiple configs -- local ones,
	// embedded ones, and remote ones.
	proxiesDispatch := func(cfg interface{}) {}
	globalDispatch := func(cfg interface{}) {}
	InitWithURLs(".", flags, &authConfig{}, proxiesDispatch, globalDispatch,
		proxyConfigURLs, globalConfigURLs, &http.Transport{})

	// sleep some amount
	globalPollInterval := 3 * time.Second
	proxyPollInterval := 1 * time.Second
	time.Sleep(6500 * time.Millisecond)
	// in 6.5 sec, should have made:
	// - 1 + (6 / 3) = 3 global requests
	// - 1 + (6 / 1) = 7 proxy requests

	// test that proxy & config servers were called the correct number of times
	finalGlobalReqCount := atomic.LoadUint64(&globalReqCount)
	finalProxyReqCount := atomic.LoadUint64(&proxyReqCount)
	assert.Equal(t, 3, int(finalGlobalReqCount), "should have fetched global config every %v", globalPollInterval)
	assert.Equal(t, 7, int(finalProxyReqCount), "should have fetched proxy config every %v", proxyPollInterval)
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
