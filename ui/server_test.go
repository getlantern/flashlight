package ui

import (
	"net"
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

	s := newServer("", "test-http-token")

	assert.Equal(t, "", urlToShow)
	s.doShow("campaign", "medium", show)

	assert.NotEqual(t, "", urlToShow)

	s.externalURL = "test"

	s.doShow("campaign", "medium", show)

	assert.Equal(t, "test", urlToShow)
}

func TestStartServer(t *testing.T) {
	startServer := func(addr *net.TCPAddr) *server {
		s := newServer("", "local-http-token")
		assert.NoError(t, s.start(addr, false), "should start server")
		return s
	}
	s := startServer(&net.TCPAddr{})
	// make sure the port is non-zero, same below
	assert.Regexp(t, "127.0.0.1:\\d{2,}$", s.listenAddr.String())
	assert.Regexp(t, "ui.lantern.io:\\d{2,}$", s.getUIAddr())
	s.stop()
	s = startServer(&net.TCPAddr{})
	assert.Regexp(t, ":\\d{2,}$", s.listenAddr.String())
	assert.Regexp(t, "ui.lantern.io:\\d{2,}$", s.getUIAddr())
	s.stop()
	s = startServer(&net.TCPAddr{Port: 0})
	assert.Regexp(t, ":\\d{2,}$", s.listenAddr.String())
	assert.Regexp(t, "ui.lantern.io:\\d{2,}$", s.getUIAddr())
	s.stop()
	s = startServer(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9897})
	assert.Equal(t, "127.0.0.1:9897", s.listenAddr.String())
	assert.Equal(t, "ui.lantern.io:9897", s.getUIAddr())
	s.stop()
	s = startServer(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	assert.Regexp(t, "127.0.0.1:\\d{2,}$", s.listenAddr.String())
	assert.Regexp(t, "ui.lantern.io:\\d{2,}$", s.getUIAddr())
	s.stop()
}

func TestStartServerAllowRemote(t *testing.T) {
	startServer := func(addr *net.TCPAddr) *server {
		s := newServer("", "local-http-token")
		assert.NoError(t, s.start(addr, true), "should start server")
		return s
	}

	s := startServer(&net.TCPAddr{})
	// make sure the port is non-zero, same below
	assert.Regexp(t, "^:\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "ui.lantern.io:\\d{2,}$", s.getUIAddr())
	s.stop()
	s = startServer(&net.TCPAddr{})
	assert.Regexp(t, "^:\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "ui.lantern.io:\\d{2,}$", s.getUIAddr())
	s.stop()
	s = startServer(&net.TCPAddr{Port: 0})
	assert.Regexp(t, "^:\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "ui.lantern.io:\\d{2,}$", s.getUIAddr())
	s.stop()
	s = startServer(&net.TCPAddr{Port: 0})
	assert.Regexp(t, "^:\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "ui.lantern.io:\\d{2,}$", s.getUIAddr())
	s.stop()
	s = startServer(&net.TCPAddr{Port: 9898})
	assert.Equal(t, ":9898", s.listenAddr.String())
	assert.Equal(t, "ui.lantern.io:9898", s.getUIAddr())
	s.stop()
	s = startServer(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	assert.Regexp(t, "^:\\d{2,}$", s.listenAddr)
	// For simplicity, ignore the case when localhost is unavailable and
	// allowRemote is true. It hardly happen in field, and skilled user can
	// work around it by replacing localhost to the address of any interfaces
	// in browser.
	assert.Regexp(t, "ui.lantern.io:\\d{2,}$", s.getUIAddr())
	s.stop()
	// Simulate the case when unable to listen on saved uiaddr.
	s = startServer(&net.TCPAddr{Port: 98980000})
	assert.NotEqual(t, ":98980000", s.listenAddr,
		"should not listen on invalid port")
	assert.Regexp(t, "^:\\d{2,}$", s.listenAddr,
		"passing invalid port should fallback to default addresses")
	assert.Regexp(t, "ui.lantern.io:\\d{2,}$", s.getUIAddr(),
		"passing invalid port should fallback to default addresses")
	s.stop()
}

func TestCheckOrigin(t *testing.T) {
	s := newServer("", "token")
	s.start(&net.TCPAddr{Port: 9898}, false)
	doTestCheckOrigin(t, s, map[string]bool{
		"localhost:9898": true,
		"localhost:1243": false,
		"127.0.0.1:9898": false,
		"anyhost:9898":   false,
	})
	s.stop()

	s.start(&net.TCPAddr{Port: 9897}, false)
	doTestCheckOrigin(t, s, map[string]bool{
		"127.0.0.1:9897": true,
		"localhost:9897": false,
		"127.0.0.1:1243": false,
		"anyhost:9897":   false,
	})
	s.stop()

	s.start(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9896}, true)
	doTestCheckOrigin(t, s, map[string]bool{
		"localhost:9896": true,
		"127.0.0.1:9896": true,
		"localhost:1243": false,
		"anyhost:9896":   true,
	})
	s.stop()
}

func doTestCheckOrigin(t *testing.T, s *server, testOrigins map[string]bool) {
	var hit bool
	var basic http.HandlerFunc = func(http.ResponseWriter, *http.Request) {
		hit = true
	}
	h := s.checkOrigin(basic)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.True(t, hit, "get / should always pass the check")

	hit = false
	req, _ = http.NewRequest("GET", "/abc", nil)
	h.ServeHTTP(w, req)
	assert.False(t, hit, "request without token should fail the check")

	hit = false
	url := util.SetURLParam("http://"+path.Join(s.accessAddr, "/abc"), "token", "wrong-token")
	req, _ = http.NewRequest("GET", url, nil)
	h.ServeHTTP(w, req)
	assert.False(t, hit, "request with incorrect token should fail the check")

	url = s.addToken("/abc")
	req, _ = http.NewRequest("GET", url, nil)
	h.ServeHTTP(w, req)
	assert.True(t, hit, "request with correct token should pass the check")

	hit = false
	req, _ = http.NewRequest("GET", "/abc", nil)
	req.Header.Set("Origin", "http://"+s.listenAddr.String()+"/")
	h.ServeHTTP(w, req)
	assert.True(t, hit, "request with the same origin should pass the check")

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
