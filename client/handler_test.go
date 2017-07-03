package client

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/getlantern/httpseverywhere"
	"github.com/getlantern/httptest"
	"github.com/stretchr/testify/assert"
)

func TestRewriteHTTPSSuccess(t *testing.T) {
	client := newClient()
	client.rewriteToHTTPS = httpseverywhere.Eager()
	w := httptest.NewRecorder(nil)
	req, _ := http.NewRequest("GET", "http://www.adaptec.com/", nil)
	client.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
	assert.Equal(t, "max-age:86400", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "https://www.adaptec.com/", resp.Header.Get("Location"))
	assert.True(t, len(resp.Header.Get("Expires")) > 0)
}

func TestRewriteHTTPSCORS(t *testing.T) {
	client := newClient()
	client.rewriteToHTTPS = httpseverywhere.Eager()
	w := httptest.NewRecorder(nil)
	req, _ := http.NewRequest("GET", "http://www.adaptec.com/", nil)
	req.Header.Set("Origin", "www.adaptec.com")
	client.ServeHTTP(w, req)
	resp := w.Result()
	assert.NotEqual(t, http.StatusMovedPermanently, resp.StatusCode)
}

func TestEasylist(t *testing.T) {
	client := newClient()
	w := httptest.NewRecorder(nil)
	req, _ := http.NewRequest("GET", "http://test.com", nil)
	client.easyblock(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestAdSwap(t *testing.T) {
	client := newClient()
	for orig, updated := range adSwapJavaScriptInjections {
		w := httptest.NewRecorder(nil)
		req, _ := http.NewRequest("GET", orig, nil)
		client.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		expectedLocation := fmt.Sprintf("%v?lang=%v&url=%v&force=true", updated, url.QueryEscape(testLang), url.QueryEscape(testAdSwapTargetURL))
		assert.Equal(t, expectedLocation, resp.Header.Get("Location"))
	}
}
