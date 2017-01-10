package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeAddr(t *testing.T) {
	endpoint := "127.0.0.1:1892"
	addr := normalizeAddr("http://" + endpoint)
	assert.Equal(t, endpoint, addr)

	addr = normalizeAddr("")
	assert.Equal(t, defaultUIAddress, addr)

	addr = normalizeAddr(endpoint)
	assert.Equal(t, endpoint, addr)
}

func getTestHandler() http.Handler {
	return getTestServer("some-token").mux
}

func getTestServer(token string) *Server {
	allowRemote := false
	s := NewServer("localhost:", allowRemote, "", token)
	attachHandlers(s, allowRemote)
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

func TestLanternPACURL(t *testing.T) {
	var rw httptest.ResponseRecorder
	s := getTestServer("token")
	req, _ := http.NewRequest("GET", s.AddToken("/proxy_on.pac"), nil)
	s.mux.ServeHTTP(&rw, req)
	assert.Equal(t, "no-cache, no-store, must-revalidate", rw.HeaderMap.Get("Cache-Control"))
}
