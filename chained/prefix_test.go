package chained

import (
	"context"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/proxyimpl"
	"github.com/stretchr/testify/require"
)

// TestPrefixImpl_Success tests that the multipath prefix dialer works as
// expected when the proxy is reachable.
//
// See "proxyimpl/prefix.go:PrefixImpl" to understand the prefix logic and
// "chained.dialer.go:multipathPrefixDialOrigin()" to see the multipath logic
func TestPrefixImpl_Success(t *testing.T) {
	// 4 bytes each that are basically
	// ['A', 'A', 'A', 'A'] and ['B', 'B', 'B', 'B']
	prefixes := []string{"41414141", "42424242"}
	// XXX <13-03-2023, soltzen> It's not necessary to have *all* prefixes be
	// the same size, but this is just to make the test "PrefixTCPListener"
	// struct easier to write. This is not necessary in production.
	prefixSize := 4

	// Init a listener that will accept connections and discard the prefix from
	// the connection before reading the rest of the message.
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	prefixLn := NewPrefixTCPListener(ln.(*net.TCPListener), prefixSize)
	// Init a server with the same listener
	srv := http.Server{
		Handler: http.HandlerFunc(
			func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		),
	}
	go srv.Serve(prefixLn)
	defer srv.Close()

	// Create the dialer
	tempConfigDir, err := ioutil.TempDir("", "test")
	require.NoError(t, err)
	defer os.RemoveAll(tempConfigDir)
	dialer, err := CreateDialer(
		tempConfigDir, "test-dialer",
		&config.ProxyConfig{
			Addr:               ln.Addr().String(),
			PluggableTransport: "http",
			Prefixes:           prefixes,
		},
		&common.UserConfigData{},
	)
	require.NoError(t, err)
	require.NotNil(t, dialer)

	// Assert we get all prefixes
	var wg sync.WaitGroup
	if prefixImpl, ok := dialer.Implementation().(*proxyimpl.PrefixImpl); ok {
		// Assert all the prefixes from config.ProxyConfig were loaded in the
		// PrefixImpl
		typedPrefixes, err := proxyimpl.NewPrefixSliceFromHexStringSlice(
			prefixes,
		)
		require.NoError(t, err)
		require.Equal(t, typedPrefixes, prefixImpl.Prefixes)

		// Add 1 to the wait group for each prefix
		for range prefixes {
			wg.Add(1)
		}

		// Assert we get all prefixes **once** and decrement the wait group for
		// each prefix
		var receivedPrefixes sync.Map
		prefixImpl.SetSuccessfulPrefixCallback(func(pr proxyimpl.Prefix) {
			// fmt.Printf("pr = %+v\n", pr)
			_, ok := receivedPrefixes.Load(pr.String())
			require.False(t, ok, "Prefix already received")
			require.Contains(t, prefixes, pr.String())
			wg.Done()
			receivedPrefixes.Store(pr.String(), true)
		})
	}

	// Run HTTP request against the server with the proxyImpl we have
	//
	// XXX <13-03-2023, soltzen> Make sure the timeout here is **bigger** than
	// 2 seconds. chained/dialer.go:doDialOrigin() checks if the context is
	// less than 2 seconds and will fail if that's the case
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx,
		"GET",
		// The URL doesn't matter: whatever we send to the test server, it will
		// just return 200OK
		"http://www.whatever.com",
		nil)
	require.NoError(t, err)
	rt := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			c, failedUpstream, err := dialer.DialContext(ctx, network, addr)
			require.NoError(t, err, "DialContext failed. Upstream failed: %v",
				failedUpstream)
			return c, err
		},
	}
	// The roundtripper should run the "http" pluggable transport using a
	// multipath dialer **and** attach a prefix to each connection.
	//
	// We're asserting the prefix and logic with the waitgroup and goroutines above:
	// - The waitgroup will be decremented for each successful prefix
	// - The successfulPrefixChan will contain all the prefixes (one for each connection)
	//
	// We're asserting the "http" pluggable transport by checking if
	// resp.StatusCode is 200 (which is what our test server should return).
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Wait for all prefixes to be dialed
	waitForWaitGroupOrTimeout(t, &wg, 5*time.Second,
		"timed out waiting for prefixes to be dialed")
}

// TeTestPrefixImpl_ProxyUnreachable tests that the multipath prefix dialer
// works as expected when the proxy is **not** reachable.
//
// It should basically return an error as expected. No prefixes should be sent
// since the prefix logic is executed **after** the initial dial handshake
// (e.g., TCP handshake) is done (if the PT uses TCP)
func TestPrefixImpl_ProxyUnreachable(t *testing.T) {
	prefixes := []string{"41414141", "42424242"}

	// Create the dialer
	tempConfigDir, err := ioutil.TempDir("", "test")
	require.NoError(t, err)
	defer os.RemoveAll(tempConfigDir)
	dialer, err := CreateDialer(
		tempConfigDir, "test-dialer",
		&config.ProxyConfig{
			// This is a non-existent server
			Addr:               "localhost:3223",
			PluggableTransport: "http",
			Prefixes:           prefixes,
		},
		&common.UserConfigData{},
	)
	require.NoError(t, err)
	require.NotNil(t, dialer)

	if prefixImpl, ok := dialer.Implementation().(*proxyimpl.PrefixImpl); ok {
		// Assert all the prefixes from config.ProxyConfig were loaded in the
		// PrefixImpl
		typedPrefixes, err := proxyimpl.NewPrefixSliceFromHexStringSlice(
			prefixes,
		)
		require.NoError(t, err)
		require.Equal(t, typedPrefixes, prefixImpl.Prefixes)

		// Assert we get **no prefixes** since the server is not reachable
		// (since prefixes are sent after the initial TCP handshake)
		prefixImpl.SetSuccessfulPrefixCallback(func(pr proxyimpl.Prefix) {
			// No prefix should arrive since the server does not exist so
			// the initial TCP handshake won't even start
			require.Fail(t, "Successful prefix arrived", pr.String())
		})
	}

	// Run HTTP request against the server with the proxyImpl we have
	//
	// XXX <13-03-2023, soltzen> Make sure the timeout here is **bigger** than
	// 2 seconds. chained/dialer.go:doDialOrigin() checks if the context is
	// less than 2 seconds and will fail if that's the case
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx,
		"GET",
		// The URL doesn't matter: whatever we send to the test server, it will
		// just return 200OK
		"http://www.whatever.com",
		nil)
	require.NoError(t, err)
	rt := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			c, failedUpstream, err := dialer.DialContext(ctx, network, addr)
			require.ErrorIs(t, err, errFailedToDialWithAnyPrefix)
			require.Nil(t, c)
			// This failure is from our client to the proxy, not from the proxy
			// to the upstream server
			require.False(t, failedUpstream)
			return c, err
		},
	}
	// The roundtripper should run the "http" pluggable transport using a
	// multipath dialer **and** attach a prefix to each connection.
	//
	// We're asserting the prefix and logic with the waitgroup and goroutines above:
	// - The waitgroup will be decremented for each successful prefix
	// - The successfulPrefixChan will contain all the prefixes (one for each connection)
	//
	// We're asserting the "http" pluggable transport by checking if
	// resp.StatusCode is 200 (which is what our test server should return).
	_, err = rt.RoundTrip(req)
	require.Error(t, err)

	// Wait to see if any prefixes arrive (they shouldn't)
	time.Sleep(3 * time.Second)
}

func waitForWaitGroupOrTimeout(
	t *testing.T,
	wg *sync.WaitGroup,
	timeout time.Duration,
	msg string,
) {
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal(msg)
	}
}
