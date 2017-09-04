package client

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/getlantern/httpseverywhere"
	"github.com/stretchr/testify/assert"
)

func TestRewriteHTTPSSuccess(t *testing.T) {
	client := newClient()
	client.rewriteToHTTPS = httpseverywhere.Eager()
	req, _ := http.NewRequest("GET", "http://www.adaptec.com/", nil)
	resp, err := roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
	assert.Equal(t, "max-age:86400", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "https://www.adaptec.com/", resp.Header.Get("Location"))
	assert.True(t, len(resp.Header.Get("Expires")) > 0)
}

func TestRewriteHTTPSCORS(t *testing.T) {
	client := newClient()
	client.rewriteToHTTPS = httpseverywhere.Eager()
	req, _ := http.NewRequest("GET", "http://www.adaptec.com/", nil)
	req.Header.Set("Origin", "www.adaptec.com")
	resp, err := roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.NotEqual(t, http.StatusMovedPermanently, resp.StatusCode)
}

func TestRewriteHTTPSRedirectLoop(t *testing.T) {
	old := httpsRewriteInterval
	defer func() { httpsRewriteInterval = old }()
	httpsRewriteInterval = 100 * time.Millisecond
	client := newClient()
	client.rewriteToHTTPS = httpseverywhere.Eager()

	req, _ := http.NewRequest("GET", "http://www.chinafile.com/", nil)
	resp, err := roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode, "should rewrite to HTTPS at first")

	req, _ = http.NewRequest("GET", "http://www.chinafile.com/", nil)
	resp, err = roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.NotEqual(t, http.StatusMovedPermanently, resp.StatusCode, "second request with same URL should not rewrite to avoid redirect loop")

	time.Sleep(2 * httpsRewriteInterval)
	req, _ = http.NewRequest("GET", "http://www.chinafile.com/", nil)
	resp, err = roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode, "should rewrite to HTTPS some time later")
}

func TestEasylist(t *testing.T) {
	client := newClient()
	req, _ := http.NewRequest("GET", "http://cdn.adblade.com", nil)
	resp, err := roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestAdSwap(t *testing.T) {
	client := newClient()
	for orig, updated := range adSwapJavaScriptInjections {
		req, _ := http.NewRequest("GET", orig, nil)
		resp, err := roundTrip(client, req)
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		expectedLocation := fmt.Sprintf("%v?lang=%v&url=%v", updated, url.QueryEscape(testLang), url.QueryEscape(testAdSwapTargetURL))
		assert.Equal(t, expectedLocation, resp.Header.Get("Location"))
	}
}
