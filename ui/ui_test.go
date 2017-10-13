package ui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStart(t *testing.T) {
	err := Start("127.0.0.1:0", "", "abcde", "ui.lantern.io", func() bool { return true })
	assert.NoError(t, err)

	uiAddr := GetUIAddr()
	assert.NotEqual(t, "", uiAddr)

	tok := AddToken("/abc")
	assert.NotEqual(t, "", tok)
	assert.True(t, strings.HasSuffix(tok, "/abc"), "Suffix not found in "+tok)
}

func TestServeFromLocalUI(t *testing.T) {
	serve = newServer("", "abcde", "ui.lantern.io", func() bool { return true })
	serve.start("127.0.0.1:0")
	req := &http.Request{
		URL: &url.URL{Host: "test.io"},
	}

	assert.Equal(t, "", req.Host)

	r := ServeFromLocalUI(req)

	assert.Equal(t, serve.listenAddr, r.Host)
	assert.Equal(t, "test.io", r.URL.Host)

	req.Method = http.MethodConnect

	r = ServeFromLocalUI(req)

	assert.Equal(t, serve.listenAddr, r.URL.Host)
}

func TestTranslations(t *testing.T) {
	serve = newServer("", "abcde", "ui.lantern.io", func() bool { return true })
	unpackUI()
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

func getTestServer(token string) *server {
	s := newServer("", token, "client.lantern.io", func() bool { return true })
	serve = s
	attachHandlers(s)
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

func TestProxyPACWithNoToken(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", "/proxy_on.pac", nil)
	getTestHandler().ServeHTTP(&rw, req)
	assert.Equal(t, "", rw.HeaderMap.Get("Cache-Control"), "Expecting no reply")
}

func TestKnownResourceWithNoOriginButWithToken(t *testing.T) {
	var rw httptest.ResponseRecorder
	s := getTestServer("token")
	req, _ := http.NewRequest("GET", s.addToken("/js/bundle.js"), nil)
	s.mux.ServeHTTP(&rw, req)
	assert.Equal(t, "no-cache, no-store, must-revalidate", rw.HeaderMap.Get("Cache-Control"))
}

func TestLanternLogoWithToken(t *testing.T) {
	var rw httptest.ResponseRecorder
	s := getTestServer("token")
	req, _ := http.NewRequest("GET", s.addToken("/img/lantern_logo.png?foo=1"), nil)
	s.mux.ServeHTTP(&rw, req)
	assert.Equal(t, "no-cache, no-store, must-revalidate", rw.HeaderMap.Get("Cache-Control"))
}

func TestLanternPACURL(t *testing.T) {
	var rw httptest.ResponseRecorder
	s := getTestServer("token")
	req, _ := http.NewRequest("GET", s.addToken("/proxy_on.pac"), nil)
	s.mux.ServeHTTP(&rw, req)
	assert.Equal(t, "no-cache, no-store, must-revalidate", rw.HeaderMap.Get("Cache-Control"))
}
