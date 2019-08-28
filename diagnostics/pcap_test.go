package diagnostics

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

	tmpDir, err := ioutil.TempDir("", "flashlight-diagnostics-pcap-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	capturing := make(chan struct{})
	captureComplete := make(chan struct{})
	go func() {
		close(capturing)
		defer func() { close(captureComplete) }()

		err = CaptureProxyTraffic(map[string]*chained.ChainedServerInfo{
			"localhost": &chained.ChainedServerInfo{Addr: s.Listener.Addr().String()},
		}, tmpDir)
		if err != nil {
			for proxyName, proxyErr := range err.(ErrorsMap) {
				t.Logf("error for %s:\n%v", proxyName, proxyErr)
			}
			t.Fatal(err)
		}
	}()

	<-capturing
	time.Sleep(time.Second)
	_, err = http.Get(s.URL)
	require.NoError(t, err)

	<-captureComplete
	pcapFile, err := os.Open(filepath.Join(tmpDir, "localhost.pcap"))
	require.NoError(t, err)
	defer pcapFile.Close()

	fileContents, err := ioutil.ReadAll(pcapFile)
	require.NoError(t, err)
	require.Contains(t, string(fileContents), serverResponseString)
}
