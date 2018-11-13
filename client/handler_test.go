package client

import (
	"net"
	"net/http"
	"testing"

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
