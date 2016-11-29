package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckOrigin(t *testing.T) {
	localHTTPToken = "a-token"
	uiaddr = "127.0.0.1:9999"
	proxiedUIAddr = proxyDomainFor(uiaddr)
	defer func() {
		uiaddr, proxiedUIAddr = "", ""
	}()

	var hit bool
	var basic http.HandlerFunc = func(http.ResponseWriter, *http.Request) {
		hit = true
	}
	h := checkOrigin(basic)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.True(t, hit, "get / should always pass the check")

	hit = false
	req, _ = http.NewRequest("GET", "/abc", nil)
	h.ServeHTTP(w, req)
	assert.False(t, hit, "request without token should fail the check")

	url := AddToken("/abc")
	req, _ = http.NewRequest("GET", url, nil)
	h.ServeHTTP(w, req)
	assert.True(t, hit, "request with correct token should pass the check")

	hit = false
	req, _ = http.NewRequest("GET", "/abc", nil)
	req.Header.Set("Origin", "http://"+uiaddr+"/")
	h.ServeHTTP(w, req)
	assert.True(t, hit, "request with the same origin should pass the check")

	hit = false
	edge = true
	PreferProxiedUI(true)
	req.Header.Set("Origin", "http://"+proxiedUIAddr+"/")
	h.ServeHTTP(w, req)
	assert.True(t, hit, "request with proxied domain as origin should pass the check")
}
