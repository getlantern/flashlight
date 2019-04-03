package client

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/getlantern/httpseverywhere"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeExoAd(t *testing.T) {
	doTestNormalizeExoAd(t, "syndication.exdynsrv.com")
	doTestNormalizeExoAd(t, "syndication.exdynsrv.com:80")
	doTestNormalizeExoAd(t, "syndication.exdynsrv.com:443")

	req, _ := http.NewRequest("GET", "http://exdynsrv.com.friendlygfw.cn", nil)
	_, ad := normalizeExoAd(req)
	assert.False(t, ad)
}

func doTestNormalizeExoAd(t *testing.T, host string) {
	req, _ := http.NewRequest("GET", "http://"+host, nil)
	qvals := req.URL.Query()
	qvals.Set("p", "https://www.somethingelse.com")
	req.URL.RawQuery = qvals.Encode()

	req2, ad := normalizeExoAd(req)
	assert.True(t, ad)
	assert.Equal(t, "https://www.getlantern.org/", req2.URL.Query().Get("p"))
}

func TestRewriteHTTPSSuccess(t *testing.T) {
	client := newClient()
	client.rewriteToHTTPS = httpseverywhere.Eager()
	req, _ := http.NewRequest("GET", "http://www.nytimes.com/", nil)
	resp, err := roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
	assert.Equal(t, "max-age:86400", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "https://www.nytimes.com/", resp.Header.Get("Location"))
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
	testURL := "http://cdn2.shoptiques.net"

	req, _ := http.NewRequest("GET", testURL, nil)
	resp, err := roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode, "should rewrite to HTTPS at first")

	req, _ = http.NewRequest("GET", testURL, nil)
	resp, err = roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.NotEqual(t, http.StatusMovedPermanently, resp.StatusCode, "second request with same URL should not rewrite to avoid redirect loop")

	time.Sleep(2 * httpsRewriteInterval)
	req, _ = http.NewRequest("GET", testURL, nil)
	resp, err = roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode, "should rewrite to HTTPS some time later")
}

func TestAdBlockingFree(t *testing.T) {
	client := newClient()
	// Give us a chance to fetch the latest easylist and lanternlist
	time.Sleep(5 * time.Second)
	// On free, nothing should be blocked
	assertAllow(t, client, "https://cdn.adblade.com", true)
}

func TestAdBlockingPro(t *testing.T) {
	client := newClientWithLangAndAdSwapTargetURL("en_US", "")
	// Give us a chance to fetch the latest easylist and lanternlist
	time.Sleep(5 * time.Second)
	assertAllow(t, client, "http://googleads.g.doubleclick.net", false)
	assertAllow(t, client, "http://www.googleadservices.com", false)
	assertAllow(t, client, "http://cdn.adblade.com", false)
}

func assertAllow(t *testing.T, client *Client, url string, allow bool) {
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}

	expectedStatus := http.StatusFound
	if !allow {
		expectedStatus = http.StatusForbidden
	}
	assert.Equal(t, expectedStatus, resp.StatusCode)
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
func TestRejectHTTPProxyPort(t *testing.T) {
	client := newClient()
	client.httpProxyIP, client.httpProxyPort, _ = net.SplitHostPort("127.0.0.1:4321")
	req, _ := http.NewRequest("GET", "http://127.0.0.1:4321", nil)
	assert.True(t, client.isHTTPProxyPort(req))
	req, _ = http.NewRequest("GET", "http://localhost:4321", nil)
	assert.True(t, client.isHTTPProxyPort(req))

	client.httpProxyIP, client.httpProxyPort, _ = net.SplitHostPort("127.0.0.1:80")
	req, _ = http.NewRequest("GET", "http://localhost", nil)
	assert.True(t, client.isHTTPProxyPort(req))
	req, _ = http.NewRequest("GET", "ws://localhost", nil)
	assert.True(t, client.isHTTPProxyPort(req))

	client.httpProxyIP, client.httpProxyPort, _ = net.SplitHostPort("127.0.0.1:443")
	req, _ = http.NewRequest("GET", "https://localhost", nil)
	assert.True(t, client.isHTTPProxyPort(req))
	req, _ = http.NewRequest("GET", "wss://localhost", nil)
	assert.True(t, client.isHTTPProxyPort(req))
}
