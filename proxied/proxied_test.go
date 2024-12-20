package proxied

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vulcand/oxy/forward"
	"gopkg.in/yaml.v2"

	"github.com/getlantern/eventual"
	"github.com/getlantern/fronted"

	"github.com/stretchr/testify/assert"

	flconfig "github.com/getlantern/flashlight/v7/config"
)

type mockChainedRT struct {
	req eventual.Value
	sc  uint32
}

func (rt *mockChainedRT) setStatusCode(code uint32) {
	atomic.StoreUint32(&rt.sc, code)
}

func (rt *mockChainedRT) statusCode() int {
	return int(atomic.LoadUint32(&rt.sc))
}

func (rt *mockChainedRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.req.Set(req)
	return &http.Response{
		Status:     fmt.Sprintf("%d OK", rt.statusCode()),
		StatusCode: rt.statusCode(),
		Body:       io.NopCloser(bytes.NewBufferString("Chained")),
	}, nil
}

type mockFrontedRT struct {
	req eventual.Value
}

func (rt *mockFrontedRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.req.Set(req)
	return &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBufferString("Fronted")),
	}, nil
}

type delayedRT struct {
	rt    http.RoundTripper
	delay time.Duration
}

func (rt *delayedRT) RoundTrip(req *http.Request) (*http.Response, error) {
	time.Sleep(rt.delay)
	return rt.rt.RoundTrip(req)
}

// TestChainedAndFrontedHeaders tests to make sure headers are correctly
// copied to the fronted request from the original chained request.
func TestChainedAndFrontedHeaders(t *testing.T) {
	updateFronted()
	directURL := "http://direct"
	req, err := http.NewRequest("GET", directURL, nil)
	if !assert.NoError(t, err) {
		return
	}
	req.Header.Set("Accept", "application/x-gzip")
	// Prevents intermediate nodes (domain-fronters) from caching the content
	req.Header.Set("Cache-Control", "no-cache")
	etag := "473892jdfda"
	req.Header.Set("X-Lantern-If-None-Match", etag)
	req.Body = io.NopCloser(bytes.NewBufferString("Hello"))

	df := &dualFetcher{&chainedAndFronted{parallel: true}, ""}
	crt := &mockChainedRT{req: eventual.NewValue(), sc: 503}
	frt := &mockFrontedRT{req: eventual.NewValue()}
	df.do(req, crt, frt)
	t.Log("Checking chained roundtripper")
	checkRequest(t, crt.req, etag, directURL)
	t.Log("Checking fronted roundtripper")
	checkRequest(t, frt.req, etag, directURL)
}

func checkRequest(t *testing.T, v eventual.Value, etag string, url string) {
	reqVal, ok := v.Get(2 * time.Second)
	if !assert.True(t, ok, "Failed to get request") {
		return
	}
	req := reqVal.(*http.Request)
	assert.Equal(t, url, req.URL.String(), "should set correct URL")
	assert.Equal(t, etag, req.Header.Get("X-Lantern-If-None-Match"), "should keep etag")
	assert.Equal(t, "no-cache", req.Header.Get("Cache-Control"), "should keep Cache-Control")
	// There should not be a host header here -- the go http client will
	// populate it automatically based on the URL.
	assert.Equal(t, "", req.Header.Get("Host"), "should remove Host from headers")
	assert.Equal(t, "", req.Header.Get("Lantern-Fronted-URL"), "should remove Lantern-Fronted-URL from headers")
	bytes, _ := ioutil.ReadAll(req.Body)
	assert.Equal(t, "Hello", string(bytes), "should pass body")
}

// TestNonIdempotentRequest tests to make sure ParallelPreferChained reject
// non-idempotent requests.
func TestNonIdempotentRequest(t *testing.T) {
	updateFronted()
	directURL := "http://direct"
	req, err := http.NewRequest("POST", directURL, nil)
	if !assert.NoError(t, err) {
		return
	}
	df := ParallelPreferChained()
	_, err = df.RoundTrip(req)
	if assert.Error(t, err, "should not send non-idempotent method in parallel") {
		assert.Contains(t, err.Error(), "attempted to use parallel round-tripper for non-idempotent method, please use ChainedThenFronted or some similar sequential round-tripper")
	}
}

// TestChainedAndFrontedParallel tests to make sure chained and fronted requests
// are both working in parallel.
func TestParallelPreferChained(t *testing.T) {
	doTestChainedAndFronted(t, ParallelPreferChained)
}

func TestChainedThenFronted(t *testing.T) {
	doTestChainedAndFronted(t, ChainedThenFronted)
}

func TestSwitchingToChained(t *testing.T) {
	updateFronted()
	chained := &mockChainedRT{req: eventual.NewValue(), sc: 503}
	fronted := &mockFrontedRT{req: eventual.NewValue()}
	req, _ := http.NewRequest("GET", "http://chained", nil)

	cf := ParallelPreferChained().(*chainedAndFronted)
	cf.getFetcher().(*dualFetcher).do(req, chained, fronted)
	time.Sleep(100 * time.Millisecond)
	_, valid := cf.getFetcher().(*dualFetcher)
	assert.True(t, valid, "should not switch fetcher if chained failed")

	chained.setStatusCode(200)
	cf.getFetcher().(*dualFetcher).do(req, &delayedRT{chained, 100 * time.Millisecond}, fronted)
	time.Sleep(100 * time.Millisecond)
	_, valid = cf.getFetcher().(*dualFetcher)
	assert.True(t, valid, "should not switch to chained fetcher if it's significantly slower")

	cf.getFetcher().(*dualFetcher).do(req, chained, &delayedRT{fronted, 100 * time.Millisecond})
	time.Sleep(100 * time.Millisecond)
	_, valid = cf.getFetcher().(*chainedRoundTripper)
	assert.True(t, valid, "should switch to chained fetcher")
}

func doTestChainedAndFronted(t *testing.T, build func() http.RoundTripper) {
	//updateFronted()
	fwd, _ := forward.New()

	sleep := 0 * time.Second

	forward := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// The sleep can help the other request to complete faster.
		time.Sleep(sleep)
		fwd.ServeHTTP(w, req)
	})

	// that's it! our reverse proxy is ready!
	s := &http.Server{
		Handler: forward,
	}

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		assert.NoError(t, err, "Unable to listen")
	}
	go s.Serve(l)

	SetProxyAddr(eventual.DefaultGetter(l.Addr().String()))

	fronter = fronted.ConfigureForTest(t)
	geo := "http://d3u5fqukq7qrhd.cloudfront.net/lookup/198.199.72.101"
	req, err := http.NewRequest("GET", geo, nil)

	assert.NoError(t, err)

	cf := build()
	resp, err := cf.RoundTrip(req)
	assert.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	//log.Debugf("Got body: %v", string(body))
	assert.True(t, strings.Contains(string(body), "United States"), "Unexpected response ")
	_ = resp.Body.Close()

	sleep = 2 * time.Second

	// Now test with a bad cloudfront url configured that won't
	// resolve and make sure even the delayed req server still gives us the result
	goodhost := "d3u5fqukq7qrhd.cloudfront.net"
	badhost := "48290.cloudfront.net"
	fronter = fronted.ConfigureHostAlaisesForTest(t, map[string]string{goodhost: badhost})

	req, err = http.NewRequest("GET", geo, nil)

	assert.NoError(t, err)
	cf = build()
	resp, err = cf.RoundTrip(req)
	assert.NoError(t, err)
	log.Debugf("Got response in test")
	body, err = io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(body), "United States"), "Unexpected response ")
	_ = resp.Body.Close()

	// Now give the bad url to the req server and make sure we still get the corret
	// result from the fronted server.
	log.Debugf("Running test with bad URL in the req server")
	bad := "http://48290.cloudfront.net/lookup/198.199.72.101"
	req, err = http.NewRequest("GET", bad, nil)
	fronter = fronted.ConfigureHostAlaisesForTest(t, map[string]string{badhost: goodhost})

	assert.NoError(t, err)
	cf = build()
	resp, err = cf.RoundTrip(req)
	if assert.NoError(t, err) {
		if assert.Equal(t, 200, resp.StatusCode) {
			body, err = io.ReadAll(resp.Body)
			if assert.NoError(t, err) {
				assert.True(t, strings.Contains(string(body), "United States"), "Unexpected response "+string(body))
			}
		}
		_ = resp.Body.Close()
	}
}

func TestChangeUserAgent(t *testing.T) {
	req, _ := http.NewRequest("GET", "abc.com", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 6.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/41.0.2228.0 Safari/537.36")
	changeUserAgent(req)
	assert.Regexp(t, "^Lantern/(.*) (.*) .*", req.Header.Get("User-Agent"))
}

// TestCloneRequestForFronted tests to make sure cloning requests is working
// correctly.
func TestCloneRequestForFronted(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://chained.com/path1?q1=test1&q2=test2", strings.NewReader("abc"))

	cf := &chainedAndFronted{
		parallel: true,
	}

	r, err := cf.cloneRequestForFronted(req)
	assert.Nil(t, err)

	dump, er := httputil.DumpRequestOut(req, true)
	assert.Nil(t, er)
	t.Logf("%v", string(dump))

	dump, er = httputil.DumpRequestOut(r, true)
	assert.Nil(t, er)
	t.Logf("%v", string(dump))

	param1 := r.URL.Query().Get("q1")
	param2 := r.URL.Query().Get("q2")
	assert.Equal(t, "test1", param1)
	assert.Equal(t, "test2", param2)

	assert.Equal(t, "chained.com", r.URL.Host)
	assert.Equal(t, "/path1", r.URL.Path)
	assert.Equal(t, req.ContentLength, r.ContentLength)
	b, _ := io.ReadAll(r.Body)
	assert.Equal(t, "abc", string(b), "should have body")
}

func updateFronted() {
	// Init domain-fronting
	global, err := os.ReadFile("../embeddedconfig/global.yaml")
	if err != nil {
		log.Errorf("Unable to load embedded global config: %v", err)
		os.Exit(1)
	}
	cfg := flconfig.NewGlobal()
	err = yaml.Unmarshal(global, cfg)
	if err != nil {
		log.Errorf("Unable to unmarshal embedded global config: %v", err)
		os.Exit(1)
	}

	certs, err := cfg.TrustedCACerts()
	if err != nil {
		log.Errorf("Unable to read trusted certs: %v", err)
	}

	tempConfigDir, err := os.MkdirTemp("", "proxied_test")
	if err != nil {
		log.Errorf("Unable to create temp config dir: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempConfigDir)
	OnNewFronts(certs, cfg.Client.FrontedProviders())
}
