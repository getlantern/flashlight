package diagnostics

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTrafficLog(t *testing.T) {
	t.Parallel()
	if !*runElevatedFlag {
		t.SkipNow()
	}

	const (
		// TODO: make TrafficLog handle concurrent captures and bump this number up
		captureAddresses     = 1
		serverResponseString = "TestCaptureProxyTraffic test server response"

		// Make the buffers large enough that we will not lose any packets.
		captureBufferSize = 1024 * 1024
		saveBufferSize    = 1024 * 1024
	)

	responseFor := func(serverNumber int) string {
		return fmt.Sprintf("%s - server number %d", serverResponseString, serverNumber)
	}

	servers := make([]*httptest.Server, captureAddresses)
	addresses := make([]string, captureAddresses)
	for i := 0; i < captureAddresses; i++ {
		resp := responseFor(i)
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintln(w, resp)
		}))
		defer servers[i].Close()
		addresses[i] = strings.Replace(servers[i].URL, "http://", "", -1)
	}

	tl, err := NewTrafficLog(addresses, captureBufferSize, saveBufferSize)
	require.NoError(t, err)

	time.Sleep(time.Second)
	for _, s := range servers {
		_, err := http.Get(s.URL)
		require.NoError(t, err)
	}

	time.Sleep(5 * time.Second)
	for _, addr := range addresses {
		require.NoError(t, tl.SaveCaptures(addr, time.Minute))
	}

	pcapFileBuf := new(bytes.Buffer)
	require.NoError(t, tl.WritePcapng(pcapFileBuf))

	pcapFile := pcapFileBuf.String()
	for i := 0; i < captureAddresses; i++ {
		require.Contains(t, pcapFile, responseFor(i))
	}
}
