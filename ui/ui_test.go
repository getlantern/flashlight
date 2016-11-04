package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeAddr(t *testing.T) {
	endpoint := "127.0.0.1:1892"
	addr, _ := normalizeAddr("http://" + endpoint)
	assert.Equal(t, endpoint, addr.String())

	addr, _ = normalizeAddr("")
	assert.Equal(t, defaultUIAddress, addr.String())

	addr, _ = normalizeAddr(endpoint)
	assert.Equal(t, endpoint, addr.String())
}

func TestStartServer(t *testing.T) {
	Start(":0", false, "", "948318")
}

func TestNoCache(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", "/", nil)
	r.ServeHTTP(&rw, req)
	assert.Equal(t, "no-cache, no-store, must-revalidate", rw.HeaderMap.Get("Cache-Control"))
}

func TestProAPI(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", "/pro/user-data", nil)
	req.Header.Set("Origin", "example.com")
	r.ServeHTTP(&rw, req)
	assert.Equal(t, "", rw.HeaderMap.Get("Cache-Control"), "The cache middleware should not be executed.")
}

func TestKnownResource(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", "/js/bundle.js", nil)
	req.Header.Set("Origin", "example.com")
	r.ServeHTTP(&rw, req)
	assert.Equal(t, "", rw.HeaderMap.Get("Cache-Control"), "The cache middleware should not be executed.")
}

func TestKnownResourceWithNoOrigin(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", "/js/bundle.js", nil)
	r.ServeHTTP(&rw, req)
	assert.Equal(t, "", rw.HeaderMap.Get("Cache-Control"), "Expecting no reply")
}

func TestProxyPACWithNoToken(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", "/proxy_on.pac", nil)
	r.ServeHTTP(&rw, req)
	assert.Equal(t, "", rw.HeaderMap.Get("Cache-Control"), "Expecting no reply")
}

func TestKnownResourceWithNoOriginButWithToken(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", AddToken("/js/bundle.js"), nil)
	r.ServeHTTP(&rw, req)
	assert.Equal(t, "no-cache, no-store, must-revalidate", rw.HeaderMap.Get("Cache-Control"))
}

func TestLanternLogoWithToken(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", AddToken("/img/lantern_logo.png?foo=1"), nil)
	r.ServeHTTP(&rw, req)
	assert.Equal(t, "no-cache, no-store, must-revalidate", rw.HeaderMap.Get("Cache-Control"))
}

func TestLanternPACURL(t *testing.T) {
	var rw httptest.ResponseRecorder
	req, _ := http.NewRequest("GET", AddToken("/proxy_on.pac"), nil)
	r.ServeHTTP(&rw, req)
	assert.Equal(t, "no-cache, no-store, must-revalidate", rw.HeaderMap.Get("Cache-Control"))
}
