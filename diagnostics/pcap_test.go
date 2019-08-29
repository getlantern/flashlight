package diagnostics

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getlantern/flashlight/chained"
	"github.com/stretchr/testify/require"
)

func TestCaptureProxyTraffic(t *testing.T) {
	t.Parallel()
	if !*runElevatedFlag {
		t.SkipNow()
	}

	const serverResponseString = "TestCaptureProxyTraffic test server response"

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, serverResponseString)
	}))
	defer s.Close()

	capturing := make(chan struct{})
	captureComplete := make(chan struct{})
	captureBuf := new(bytes.Buffer)
	go func() {
		close(capturing)
		defer func() { close(captureComplete) }()

		proxies := map[string]*chained.ChainedServerInfo{
			"localhost": &chained.ChainedServerInfo{Addr: s.Listener.Addr().String()},
		}
		cfg := CaptureConfig{
			StopChannel: CloseAfter(time.Second),
			Output:      captureBuf,
		}
		if err := CaptureProxyTraffic(proxies, &cfg); err != nil {
			for proxyName, proxyErr := range err.(ErrorsMap) {
				t.Logf("error for %s:\n%v", proxyName, proxyErr)
			}
			t.Fatal(err)
		}
	}()

	<-capturing
	time.Sleep(500 * time.Millisecond)
	_, err := http.Get(s.URL)
	require.NoError(t, err)

	<-captureComplete
	require.Contains(t, captureBuf.String(), serverResponseString)
}
