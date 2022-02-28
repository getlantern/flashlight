package quicproxy

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReverseProxy(t *testing.T) {
	t.Run("Assert that a non-CONNECT http method is rejected", func(t *testing.T) {
		// Run a reverse proxy
		errChan := make(chan error)
		srv, err := NewReverseProxy(
			":0", // addr
			testPemEncodedCert,
			testPemEncodedPrivKey,
			true, // verbose
			errChan,
		)
		require.NoError(t, err)
		go func() {
			err := <-errChan
			require.NoError(t, err)
		}()

		// Run a GET request to that reverse proxy. This should fail with a
		// specific status code since this reverse proxy **only** accepts
		// CONNECT requests
		req, err := http.NewRequest("GET", "http://localhost:"+strconv.Itoa(srv.Port), nil)
		cl := &http.Client{
			Transport: &http.Transport{
				// This is how QuicForwardProxy talks to QuicReverseProxy
				Dial: NewQuicDialer(true).Dial,
			},
		}
		resp, err := cl.Do(req)
		require.NoError(t, err)
		b, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		// It should return this status code and no body
		require.Equal(t, http.StatusTeapot, resp.StatusCode)
		require.Empty(t, b)
	})
}
