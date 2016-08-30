package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStartServer(t *testing.T) {
	Start(":0", false, "")
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
	assert.Equal(t, "no-cache, no-store, must-revalidate", rw.HeaderMap.Get("Cache-Control"), "We're not providing Origin, that means this request wasn't made from a browser.")
}
