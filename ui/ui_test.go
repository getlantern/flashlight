package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	s := newServer("", token, "client.lantern.io")
	serve = s
	attachHandlers(s)
	s.start("localhost:")
	return s
}

func TestNoCache(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", "/", nil)
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
