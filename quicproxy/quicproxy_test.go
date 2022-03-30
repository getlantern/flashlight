package quicproxy

import (
	"context"
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
		true, // verbose
		true, // insecureSkipVerify
	)
	require.NoError(t, err)
	require.NoError(t, forwardProxySrv.Run(0, errChan))
	go func() {
		err := <-errChan
		require.NoError(t, err)
	}()

	t.Run("with reverse proxy. Should succeed", func(t *testing.T) {
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
			Timeout: 5 * time.Second,
		}
		resp, err := cl.Do(req)
		require.NoError(t, err)
		b, err := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()
		require.NoError(t, err)
		require.Contains(t, string(b), "Google is built by a large team of engineers")
		require.NoError(t, reverseProxySrv.Shutdown(context.Background()))
	})

	t.Run("without reverse proxy. Should fail", func(t *testing.T) {
		req, err := http.NewRequest("GET", "https://www.google.com/humans.txt", nil)
		require.NoError(t, err)
		proxyUrl, err := url.Parse("http://localhost:" + strconv.Itoa(forwardProxySrv.Port))
		require.NoError(t, err)
		cl := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyUrl),
			},
			Timeout: 50 * time.Second,
		}
		_, err = cl.Do(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Bad Gateway")
	})
}
