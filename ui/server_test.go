package ui

import (
	"net/http"
	"net/http/httptest"
	"path"
	"testing"
	"time"

	"github.com/getlantern/flashlight/util"
	"github.com/stretchr/testify/assert"
)

func TestDoShow(t *testing.T) {
	var urlToShow string
	show := func(u string, t time.Duration) {
		urlToShow = u
	}

	s := newServer("", "test-http-token", "client.lantern.io")

	assert.Equal(t, "", urlToShow)
	s.doShow(s.rootURL(), "campaign", "medium", show)

	assert.NotEqual(t, "", urlToShow)

	s.externalURL = "test"

	s.doShow(s.rootURL(), "campaign", "medium", show)

	assert.Equal(t, "test", urlToShow)
}

func TestStartServer(t *testing.T) {
	startServer := func(addr string) *server {
		s := newServer("", "local-http-token", "client.lantern.io")
		assert.NoError(t, s.start(addr), "should start server")
		return s
	}
	s := startServer("")
	// make sure the port is non-zero, same below
	assert.Regexp(t, "localhost:\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "localhost:\\d{2,}$", s.getUIAddr())
	s.stop()
	s = startServer(":")
	assert.Regexp(t, ":\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "localhost:\\d{2,}$", s.getUIAddr())
	s.stop()
	s = startServer(":0")
	assert.Regexp(t, ":\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "localhost:\\d{2,}$", s.getUIAddr())
	s.stop()
	s = startServer("localhost:0")
	assert.Regexp(t, "localhost:\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "localhost:\\d{2,}$", s.getUIAddr())
	s.stop()
	s = startServer("localhost:9898")
	assert.Equal(t, "localhost:9898", s.listenAddr)
	assert.Equal(t, "localhost:9898", s.getUIAddr())
	s.stop()
	s = startServer("127.0.0.1:9897")
	assert.Equal(t, "127.0.0.1:9897", s.listenAddr)
	assert.Equal(t, "127.0.0.1:9897", s.getUIAddr())
	s.stop()
	s = startServer("127.0.0.1:0")
	assert.Regexp(t, "127.0.0.1:\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "127.0.0.1:\\d{2,}$", s.getUIAddr())
	s.stop()

	// Simulate the case when unable to listen on saved uiaddr.
	s = startServer("invalid-addr:9898")
	assert.NotEqual(t, "invalid-addr:9898", s.listenAddr,
		"should not listen on invalid host")
	assert.Regexp(t, "localhost:\\d{2,}$", s.listenAddr,
		"passing invalid port should fallback to default addresses")
	assert.Regexp(t, "localhost:\\d{2,}$", s.getUIAddr(),
		"passing invalid port should fallback to default addresses")
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
	assert.Regexp(t, "127.0.0.1:\\d{2,}$", s.getUIAddr(),
		"passing invalid port should fallback to default addresses")
	s.stop()
}

func TestCheckOrigin(t *testing.T) {
	s := newServer("", "token", "client.lantern.io")
	s.start("localhost:9898")
	doTestCheckRequestPath(t, s, map[string]bool{
		"localhost:9898": false,
		"localhost:1243": false,
		"127.0.0.1:9898": false,
		"anyhost:9898":   false,
	})
	s.stop()

	s.start("127.0.0.1:9897")
	doTestCheckRequestPath(t, s, map[string]bool{
		"127.0.0.1:9897": false,
		"localhost:9897": false,
		"127.0.0.1:1243": false,
		"anyhost:9897":   false,
	})
	s.stop()
}

func doTestCheckRequestPath(t *testing.T, s *server, testOrigins map[string]bool) {
	var hit bool
	var basic http.HandlerFunc = func(http.ResponseWriter, *http.Request) {
		hit = true
	}
	h := s.checkRequestPath(basic)

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

	url = s.addToken("abc")
	req, _ = http.NewRequest("GET", url, nil)
	h.ServeHTTP(w, req)
	assert.True(t, hit, "request with correct token should pass the check")

	hit = false
	req, _ = http.NewRequest("GET", "/abc", nil)
	req.Header.Set("Origin", "http://"+s.listenAddr+"/")
	h.ServeHTTP(w, req)
	assert.False(t, hit, "request with the same origin should not pass the check")

	for origin, allow := range testOrigins {
		hit = false
		req, _ = http.NewRequest("GET", "/abc", nil)
		req.Header.Set("Origin", "http://"+origin+"/")
		h.ServeHTTP(w, req)
		if allow {
			assert.True(t, hit, "origin "+origin+" should pass the check")
		} else {
			assert.False(t, hit, "origin "+origin+" should not pass the check")
		}
	}
}
