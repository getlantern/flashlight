package diagnostics

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

	tmpDir, err := ioutil.TempDir("", "flashlight-diagnostics-pcap-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	capturing := make(chan struct{})
	captureComplete := make(chan struct{})
	go func() {
		close(capturing)
		defer func() { close(captureComplete) }()

		err = CaptureProxyTraffic(map[string]*chained.ChainedServerInfo{
			// The port is ignored, so this will capture all localhost traffic.
			"localhost": &chained.ChainedServerInfo{Addr: "127.0.0.1:999"},
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
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, serverResponseString)
	}))
	defer s.Close()
	_, err = http.Get(s.URL)
	require.NoError(t, err)

	<-captureComplete
	localhostPcap, err := os.Open(filepath.Join(tmpDir, "localhost.pcap"))
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := exec.Command(
		"tshark",
		"-r", localhostPcap.Name(),
		"-T", "fields",
		"-e", "text",
	)
	cmd.Stdout, cmd.Stderr = buf, buf
	require.NoError(t, cmd.Run())
	require.Contains(t, buf.String(), serverResponseString)
}
