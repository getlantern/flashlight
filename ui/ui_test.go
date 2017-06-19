package ui

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListenTCP(t *testing.T) {
	addr := &net.TCPAddr{Port: 0}
	_, err := net.ListenTCP("tcp4", addr)

	assert.NoError(t, err, "unexpected error")
}

func TestAddrCandidates(t *testing.T) {
	endpoint := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1892}
	candidates := addrCandidates(endpoint, false)
	assert.Equal(t, append([]*net.TCPAddr{endpoint}, defaultUIAddresses...), candidates)

	candidates = addrCandidates(endpoint, false)
	assert.Equal(t, append([]*net.TCPAddr{endpoint}, defaultUIAddresses...), candidates)

	candidates = addrCandidates(endpoint, true)
	assert.Equal(t, []*net.TCPAddr{&net.TCPAddr{Port: 1892}, &net.TCPAddr{Port: 0}}, candidates)

	candidates = addrCandidates(&net.TCPAddr{}, true)
	assert.Equal(t, []*net.TCPAddr{&net.TCPAddr{Port: 0}}, candidates)

	candidates = addrCandidates(&net.TCPAddr{}, false)
	assert.Equal(t, defaultUIAddresses, candidates)
}

func getTestHandler() http.Handler {
	return getTestServer("some-token").mux
}

func getTestServer(token string) *server {
	allowRemote := false
	s := newServer("", token)
	attachHandlers(s, allowRemote)
	s.start(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, allowRemote)
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
