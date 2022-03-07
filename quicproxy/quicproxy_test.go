package quicproxy

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQuicProxy(t *testing.T) {
	errChan := make(chan error)
	forwardProxySrv, err := NewForwardProxy(
		"0",  // port
		true, // verbose
		true, // insecureSkipVerify
		errChan,
	)
	require.NoError(t, err)
	go func() {
		err := <-errChan
		require.NoError(t, err)
	}()

	reverseProxySrv, err := NewReverseProxy(
		":0", // addr
		testPemEncodedCert,
		testPemEncodedPrivKey,
		true, // verbose
		errChan,
	)
	require.NoError(t, err)
	forwardProxySrv.SetReverseProxyUrl("http://localhost:" + strconv.Itoa(reverseProxySrv.Port))

	req, err := http.NewRequest("GET", "https://www.google.com/humans.txt", nil)
	require.NoError(t, err)
	proxyUrl, err := url.Parse("http://localhost:" + strconv.Itoa(forwardProxySrv.Port))
	require.NoError(t, err)
	cl := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyUrl),
		},
	}
	time.Sleep(100 * time.Second)
	resp, err := cl.Do(req)
	require.NoError(t, err)
	b, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	require.NoError(t, err)
	require.Contains(t, string(b), "Google is built by a large team of engineers")
}
