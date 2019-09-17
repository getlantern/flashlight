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
		captureAddresses     = 10
		serverResponseString = "TestTrafficLog test server response"

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

	tl := NewTrafficLog(captureBufferSize, saveBufferSize)
	require.NoError(t, tl.UpdateAddresses(addresses))
	defer tl.Close()

	time.Sleep(500 * time.Millisecond)
	for _, s := range servers {
		_, err := http.Get(s.URL)
		require.NoError(t, err)
	}

	time.Sleep(500 * time.Millisecond)
	for _, addr := range addresses {
		require.NoError(t, tl.SaveCaptures(addr, time.Minute))
	}

	pcapFileBuf := new(bytes.Buffer)
	require.NoError(t, tl.WritePcapng(pcapFileBuf))

	pcapFile := pcapFileBuf.String()
	for i := 0; i < captureAddresses; i++ {
		requireContainsOnce(t, pcapFile, responseFor(i))
	}
}

func requireContainsOnce(t *testing.T, s, substring string) {
	t.Helper()

	b, subslice := []byte(s), []byte(substring)
	idx := bytes.Index(b, subslice)
	if idx < 0 {
		t.Fatalf("subslice does not appear")
	}
	if bytes.Index(b[idx+len(subslice):], subslice) > 0 {
		t.Fatalf("subslice appears more than once")
	}
}
