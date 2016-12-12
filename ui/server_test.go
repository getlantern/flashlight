package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStartServer(t *testing.T) {
	startServer := func(addr string) *Server {
		s := NewServer(addr, false, "", "local-http-token")
		assert.NoError(t, s.Start(), "should start server")
		return s
	}
	s := startServer("")
	assert.Regexp(t, "localhost:\\d+$", s.GetUIAddr())
	s.Stop()
	s = startServer(":0")
	assert.Regexp(t, "localhost:\\d+$", s.GetUIAddr())
	s.Stop()
	s = startServer("localhost:0")
	assert.Regexp(t, "localhost:\\d+$", s.GetUIAddr())
	s.Stop()
	s = startServer("localhost:9898")
	assert.Equal(t, "localhost:9898", s.GetUIAddr())
	s.Stop()
	s = startServer("127.0.0.1:0")
	assert.Regexp(t, "127.0.0.1:\\d+$", s.GetUIAddr())
	s.Stop()
}

func TestCheckOrigin(t *testing.T) {
	s := NewServer("localhost:9898", false, "", "token")
	var hit bool
	var basic http.HandlerFunc = func(http.ResponseWriter, *http.Request) {
		hit = true
	}
	h := checkOrigin(basic, s.localHTTPToken, s.listenAddr)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.True(t, hit, "get / should always pass the check")

	hit = false
	req, _ = http.NewRequest("GET", "/abc", nil)
	h.ServeHTTP(w, req)
	assert.False(t, hit, "request without token should fail the check")

	url := s.AddToken("/abc")
	req, _ = http.NewRequest("GET", url, nil)
	h.ServeHTTP(w, req)
	assert.True(t, hit, "request with correct token should pass the check")

	hit = false
	req, _ = http.NewRequest("GET", "/abc", nil)
	req.Header.Set("Origin", "http://"+s.listenAddr+"/")
	h.ServeHTTP(w, req)
	assert.True(t, hit, "request with the same origin should pass the check")
}
