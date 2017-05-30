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

	s := newServer("127.0.0.1:7777", false, "", "test-http-token")

	assert.Equal(t, "", urlToShow)
	s.doShow("campaign", "medium", show)

	assert.NotEqual(t, "", urlToShow)

	s.externalURL = "test"

	s.doShow("campaign", "medium", show)

	assert.Equal(t, "test", urlToShow)
}

func TestStartServer(t *testing.T) {
	startServer := func(addr string) *server {
		s := newServer(addr, false, "", "local-http-token")
		assert.NoError(t, s.start(), "should start server")
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
	s = startServer("127.0.0.1:0")
	assert.Regexp(t, "127.0.0.1:\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "127.0.0.1:\\d{2,}$", s.getUIAddr())
	s.stop()
}

func TestStartServerAllowRemote(t *testing.T) {
	startServer := func(addr string) *server {
		s := newServer(addr, true, "", "local-http-token")
		assert.NoError(t, s.start(), "should start server")
		return s
	}
	s := startServer("")
	// make sure the port is non-zero, same below
	assert.Regexp(t, ":\\d{2,}$", s.listenAddr)
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
	assert.Regexp(t, ":\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "localhost:\\d{2,}$", s.getUIAddr())
	s.stop()
	s = startServer("localhost:9898")
	assert.Equal(t, ":9898", s.listenAddr)
	assert.Equal(t, "localhost:9898", s.getUIAddr())
	s.stop()
	s = startServer("127.0.0.1:0")
	assert.Regexp(t, ":\\d{2,}$", s.listenAddr)
	assert.Regexp(t, "localhost:\\d{2,}$", s.getUIAddr())
	s.stop()
}

func TestCheckOrigin(t *testing.T) {
	localhost := newServer("localhost:9898", false, "", "token")
	doTestCheckOrigin(t, localhost, map[string]bool{
		"localhost:9898": true,
		"localhost:1243": false,
		"127.0.0.1:9898": false,
		"anyhost:9898":   false,
	})

	localIP := newServer("127.0.0.1:9898", false, "", "token")
	doTestCheckOrigin(t, localIP, map[string]bool{
		"127.0.0.1:9898": true,
		"localhost:9898": false,
		"127.0.0.1:1243": false,
		"anyhost:9898":   false,
	})

	allowRemote := newServer("localhost:9898", true, "", "token")
	doTestCheckOrigin(t, allowRemote, map[string]bool{
		"localhost:9898": true,
		"127.0.0.1:9898": true,
		"localhost:1243": false,
		"anyhost:9898":   true,
	})
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
	req.Header.Set("Origin", "http://"+s.listenAddr+"/")
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
