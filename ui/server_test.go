package ui

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/util"
	"github.com/stretchr/testify/assert"
)

func TestDoShow(t *testing.T) {
	var urlToShow string
	show := func(u string, t time.Duration) {
		urlToShow = u
	}

	s := newServer("", "local-http-token", false)

	assert.Equal(t, "", urlToShow)
	s.doShow(s.rootURL(), "campaign", "medium", show)

	assert.NotEqual(t, "", urlToShow)

	s.externalURL = "test"

	s.doShow(s.rootURL(), "campaign", "medium", show)

	assert.Equal(t, "test", urlToShow)
}

func TestListen(t *testing.T) {
	prohibitedPortInt := 2049
	prohibitedPort := strconv.Itoa(prohibitedPortInt)
	prohibitedPortAddr := fmt.Sprintf("localhost:%v", prohibitedPort)

	// the listen function will choose a non-prohibited port when there's a backup candidate
	{
		l, _, err := listen([]string{prohibitedPortAddr, "localhost:0"})
		assert.Nil(t, err)
		actualPort := l.Addr().(*net.TCPAddr).Port
		assert.NotEqual(t, actualPort, prohibitedPortInt)
		l.Close()
	}

	// the listen function will return an error if *only* a prohibited port address
	// is provided
	{
		_, _, err := listen([]string{prohibitedPortAddr})
		assert.NotNil(t, err)
	}
}

func TestStartServer(t *testing.T) {
	startServer := func(addr string) *Server {
		s := newServer("", "test-http-token", false)
		assert.NoError(t, s.start(addr), "should start server")
		return s
	}
	s := startServer("")
	// make sure the port is non-zero, same below
	assert.Regexp(t, "localhost:\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "localhost:\\d{2,}$", s.GetUIAddr())
	s.stop()
	s = startServer(":")
	assert.Regexp(t, ":\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "localhost:\\d{2,}$", s.GetUIAddr())
	s.stop()
	s = startServer(":0")
	assert.Regexp(t, ":\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "localhost:\\d{2,}$", s.GetUIAddr())
	s.stop()
	s = startServer("localhost:0")
	assert.Regexp(t, "localhost:\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "localhost:\\d{2,}$", s.GetUIAddr())
	s.stop()
	s = startServer("localhost:9898")
	assert.Equal(t, "localhost:9898", s.listenAddr)
	assert.Equal(t, "localhost:9898", s.GetUIAddr())
	s.stop()
	s = startServer("127.0.0.1:9897")
	assert.Equal(t, "127.0.0.1:9897", s.listenAddr)
	assert.Equal(t, "127.0.0.1:9897", s.GetUIAddr())
	s.stop()
	s = startServer("127.0.0.1:0")
	assert.Regexp(t, "127.0.0.1:\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "127.0.0.1:\\d{2,}$", s.GetUIAddr())
	s.stop()

	// Simulate the case when unable to listen on saved uiaddr.
	s = startServer("invalid-addr:9898")
	assert.NotEqual(t, "invalid-addr:9898", s.listenAddr,
		"should not listen on invalid host")
	assert.Regexp(t, "localhost:\\d{2,}$", s.listenAddr,
		"passing invalid port should fallback to default addresses")
	assert.Regexp(t, "localhost:\\d{2,}$", s.GetUIAddr(),
		"passing invalid port should fallback to default addresses")
	s.stop()

	// Ensure that UI won't start on chrome restricted ports
	var port int
	// This test is non-deterministic in that it may not catch incorrect code because
	// net.Listen is overwhelmingly likely to choose a port which is not prohibited
	// However, it will never report a test failure if the code is behaving correctly
	s = startServer("127.0.0.1:0")
	port = s.listener.Addr().(*net.TCPAddr).Port
	assert.False(t, prohibitedPorts[port])
	s.stop()

	// This test is deterministic
	s = startServer("127.0.0.1:2049")
	port = s.listener.Addr().(*net.TCPAddr).Port
	assert.False(t, prohibitedPorts[port])
	s.stop()

	oldDefault := defaultUIAddresses
	defer func() { defaultUIAddresses = oldDefault }()
	// Simulate the case when unable to listen on localhost.
	defaultUIAddresses = []string{"localhost:999999", "127.0.0.1:0"}
	s = startServer("invalid-addr:9898")
	assert.NotEqual(t, "invalid-addr:9898", s.listenAddr,
		"should not listen on invalid host")
	assert.Regexp(t, "127.0.0.1:\\d{2,}$", s.listenAddr,
		"passing invalid port should fallback to default addresses")
	assert.Regexp(t, "127.0.0.1:\\d{2,}$", s.GetUIAddr(),
		"passing invalid port should fallback to default addresses")
	s.stop()
}

func TestCheckOrigin(t *testing.T) {
	s := newServer("", "token", false)
	s.start("localhost:9898")
	doTestCheckRequestToken(t, s, map[*http.Request]bool{
		newRequest("http://localhost:9898"): false,
		newRequest("http://localhost:1243"): false,
		newRequest("http://127.0.0.1:9898"): false,
		newRequest("http://anyhost:9898"):   false,
	})
	s.stop()

	s.start("127.0.0.1:9897")
	doTestCheckRequestToken(t, s, map[*http.Request]bool{
		newRequest("http://127.0.0.1:9897"):              false,
		newRequest("http://localhost:9897"):              false,
		newRequest("http://127.0.0.1:1243"):              false,
		newRequest("http://anyhost:9897"):                false,
		newRequest("http://localhost:9898/token"):        true,
		newRequest("http://127.0.0.1:9898/testesttoken"): true,
	})
	s.stop()
}

func newRequest(url string) *http.Request {
	r, _ := http.NewRequest("GET", url, nil)
	r.Header.Set("referer", url)
	return r
}

func doTestCheckRequestToken(t *testing.T, s *Server, testOrigins map[*http.Request]bool) {
	var hit bool
	var basic http.HandlerFunc = func(http.ResponseWriter, *http.Request) {
		hit = true
	}
	h := checkRequestForToken(basic, s.localHTTPToken)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.False(t, hit, "naked path should not pass")

	hit = false
	req, _ = http.NewRequest("GET", "/abc", nil)
	h.ServeHTTP(w, req)
	assert.False(t, hit, "request without proper path should fail the check")

	hit = false
	url := util.SetURLParam("http://"+path.Join(s.accessAddr, "/abc"), "token", "wrong-token")
	req, _ = http.NewRequest("GET", url, nil)
	h.ServeHTTP(w, req)
	assert.False(t, hit, "request with incorrect token should fail the check")

	url = s.AddToken("abc")
	req, _ = http.NewRequest("GET", url, nil)
	req.Header.Set("referer", url)
	h.ServeHTTP(w, req)
	assert.True(t, hit, "request with correct token should pass the check")

	hit = false
	req, _ = http.NewRequest("GET", "/abc", nil)
	h.ServeHTTP(w, req)
	assert.False(t, hit, "request with the same origin should not pass the check")

	for req, allow := range testOrigins {
		hit = false
		h.ServeHTTP(w, req)
		if allow {
			assert.True(t, hit, "origin "+req.URL.String()+" should pass the check")
		} else {
			assert.False(t, hit, "origin "+req.URL.String()+" should not pass the check")
		}
	}
}

func TestStart(t *testing.T) {
	serve, err := StartServer("127.0.0.1:0", "", "abcde",
		false, &PathHandler{Pattern: "/testing", Handler: http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			resp.WriteHeader(http.StatusOK)
		})})
	assert.NoError(t, err)

	uiAddr := serve.GetUIAddr()
	assert.NotEqual(t, "", uiAddr)

	tok := serve.AddToken("/abc")
	assert.NotEqual(t, "", tok)
	assert.True(t, strings.HasSuffix(tok, "/abc"), "Suffix not found in "+tok)
}

func TestTranslations(t *testing.T) {
	dat, err := Translations("en-US.json")
	assert.NoError(t, err, "Could not fetch locale")
	assert.NotNil(t, dat)
}

func TestAddrCandidates(t *testing.T) {
	endpoint := "127.0.0.1:1892"
	candidates := addrCandidates("http://" + endpoint)
	assert.Equal(t, append([]string{endpoint}, defaultUIAddresses...), candidates)

	candidates = addrCandidates(endpoint)
	assert.Equal(t, append([]string{endpoint}, defaultUIAddresses...), candidates)

	candidates = addrCandidates("")
	assert.Equal(t, defaultUIAddresses, candidates)
}

func getTestHandler() http.Handler {
	return getTestServer("some-token").mux
}

func getTestServer(token string) *Server {
	s := newServer("", token, false)
	s.start("localhost:")
	return s
}

func TestNoCache(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", "/some-token/", nil)
	getTestHandler().ServeHTTP(&rw, req)
	assert.Equal(t, "no-cache, no-store, must-revalidate", rw.HeaderMap.Get("Cache-Control"))
}

func TestProAPI(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", "/pro/user-data", nil)
	req.Header.Set("Origin", "http://example.com")
	getTestHandler().ServeHTTP(&rw, req)
	assert.Equal(t, "", rw.HeaderMap.Get("Cache-Control"), "The cache middleware should not be executed.")
}

func TestKnownResource(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", "/js/bundle.js", nil)
	req.Header.Set("Origin", "http://example.com")
	getTestHandler().ServeHTTP(&rw, req)
	assert.Equal(t, "", rw.HeaderMap.Get("Cache-Control"), "The cache middleware should not be executed.")
}

func TestKnownResourceWithNoOrigin(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", "/js/bundle.js", nil)
	getTestHandler().ServeHTTP(&rw, req)
	assert.Equal(t, "", rw.HeaderMap.Get("Cache-Control"), "Expecting no reply")
}

func TestKnownResourceWithNoOriginButWithToken(t *testing.T) {
	var rw httptest.ResponseRecorder
	s := getTestServer("token")
	req, _ := http.NewRequest("GET", s.AddToken("/js/bundle.js"), nil)
	s.mux.ServeHTTP(&rw, req)
	assert.Equal(t, "no-cache, no-store, must-revalidate", rw.HeaderMap.Get("Cache-Control"))
}

func TestLanternLogoWithToken(t *testing.T) {
	var rw httptest.ResponseRecorder
	s := getTestServer("token")
	req, _ := http.NewRequest("GET", s.AddToken("/img/lantern_logo.png?foo=1"), nil)
	s.mux.ServeHTTP(&rw, req)
	assert.Equal(t, "no-cache, no-store, must-revalidate", rw.HeaderMap.Get("Cache-Control"))
}

func TestAddCampaign(t *testing.T) {
	startURL := "https://test.com"
	campaignURL, err := AddCampaign(startURL, "test-campaign", "test-content", "test-medium")
	assert.NoError(t, err, "unexpected error")
	assert.Equal(t, "https://test.com?utm_campaign=test-campaign&utm_content=test-content&utm_medium=test-medium&utm_source="+common.Platform, campaignURL)

	// Now test a URL that will produce an error
	startURL = ":"
	_, err = AddCampaign(startURL, "test-campaign", "test-content", "test-medium")
	assert.Error(t, err)
}
