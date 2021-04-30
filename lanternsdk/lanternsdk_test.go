package lanternsdk

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	"github.com/stretchr/testify/require"
)

func TestProxying(t *testing.T) {
	configDir, err := ioutil.TempDir("", "publicsdk_test")
	require.NoError(t, err)
	defer os.RemoveAll(configDir)

	result, err := Start("lanternSDKtest", configDir, "en_US", 10000)
	require.NoError(t, err, "Should have been able to start lantern")
	newResult, err := Start("lanternSDKtest", "testapp", "en_US", 10000)
	require.NoError(t, err, "Should have been able to start lantern twice")
	require.Equal(t, result.HTTPAddr, newResult.HTTPAddr, "2nd start should have resulted in the same address")
	testProxiedRequest(t, result.HTTPAddr, false)
}

func testProxiedRequest(t *testing.T, proxyAddr string, socks bool) {
	target := "https://www.google.com/humans.txt"

	req, _ := http.NewRequest(http.MethodGet, target, nil)

	transport := &http.Transport{}
	if socks {
		// Set up SOCKS proxy
		proxyURL, err := url.Parse("socks5://" + proxyAddr)
		require.NoError(t, err)

		socksDialer, err := proxy.FromURL(proxyURL, proxy.Direct)
		require.NoError(t, err)
		transport.Dial = socksDialer.Dial
	} else {
		proxyURL, _ := url.Parse("http://" + proxyAddr)
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	client := &http.Client{
		Timeout:   time.Second * 15,
		Transport: transport,
	}

	res, err := client.Do(req)
	require.NoError(t, err)

	buf, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, "Google is built by a large team of engineers, designers, researchers, robots, and others in many different sites across the globe. It is updated continuously, and built with more tools and technologies than we can shake a stick at. If you'd like to help us out, see careers.google.com.\n", string(buf))
}
