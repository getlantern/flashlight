package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getlantern/flashlight/ops"
	"github.com/stretchr/testify/assert"
)

func TestRedirect(t *testing.T) {
	op := ops.Begin("client-test")
	defer op.End()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "http://test.com", nil)
	httpsURL := "https://test.com"
	newClient().redirect(w, req, httpsURL, op)

	resp := w.Result()

	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
	assert.Equal(t, "max-age:86400", resp.Header.Get("Cache-Control"))
	assert.True(t, len(resp.Header.Get("Expires")) > 0)
}

func TestEasylist(t *testing.T) {
	client := newClient()
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
