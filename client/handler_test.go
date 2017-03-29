package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getlantern/flashlight/ops"
	"github.com/stretchr/testify/assert"
)

func TestRedirect(t *testing.T) {
	client := &Client{}
	op := ops.Begin("client-test")
	defer op.End()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "http://test.com", nil)
	httpsURL := "https://test.com"
	client.redirect(w, req, httpsURL, op)

	resp := w.Result()

	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
}

func TestEasylist(t *testing.T) {
	client := &Client{}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "http://test.com", nil)
	client.easyblock(w, req, true)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "http://test.com", nil)
	client.easyblock(w, req, false)
	resp = w.Result()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
