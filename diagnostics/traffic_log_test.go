package diagnostics

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTrafficLog(t *testing.T) {
	if !*runElevatedFlag {
		t.SkipNow()
	}

	const (
		captureAddresses     = 10
		serverResponseString = "TestTrafficLog test server response"

		// Make the buffers large enough that we will not lose any packets.
		captureBufferSize = 1024 * 1024
		saveBufferSize    = 1024 * 1024

		// The time we allow for capture to start or take place.
		captureWaitTime = 200 * time.Millisecond
	)

	originalFlagMTULimit := *FlagMTULimit
	*FlagMTULimit = false
	defer func() { *FlagMTULimit = originalFlagMTULimit }()

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

	time.Sleep(captureWaitTime)
	for _, s := range servers {
		_, err := http.Get(s.URL)
		require.NoError(t, err)
	}

	time.Sleep(captureWaitTime)
	for _, addr := range addresses {
		tl.SaveCaptures(addr, time.Minute)
	}

	pcapFileBuf := new(bytes.Buffer)
	require.NoError(t, tl.WritePcapng(pcapFileBuf))

	pcapFile := pcapFileBuf.String()
	for i := 0; i < captureAddresses; i++ {
		requireContainsOnce(t, pcapFile, responseFor(i))
	}
}

func TestStatsTracker(t *testing.T) {
	t.Parallel()

	const (
		channels          = 10
		sendsPerChannel   = 5
		receivedPerSend   = uint64(10)
		droppedPerSend    = uint64(3)
		sleepBetweenSends = 10 * time.Millisecond
	)

	st := newStatsTracker()
	st.output = make(chan CaptureStats, channels*sendsPerChannel)

	wg := new(sync.WaitGroup)
	for i := 0; i < channels; i++ {
		c := make(chan CaptureStats)
		wg.Add(2)
		go func() {
			defer wg.Done()

			var received, dropped uint64
			for s := 0; s < sendsPerChannel; s++ {
				received = received + receivedPerSend
				dropped = dropped + droppedPerSend
				c <- CaptureStats{received, dropped}
			}
			close(c)
		}()
		go func() { st.track(c); wg.Done() }()
	}
	wg.Wait()
	close(st.output)

	var received, dropped uint64
	for stats := range st.output {
		newlyReceived := stats.Received - received
		newlyDropped := stats.Dropped - dropped
		require.Equal(t, receivedPerSend, newlyReceived)
		require.Equal(t, droppedPerSend, newlyDropped)
		received, dropped = stats.Received, stats.Dropped
	}
	require.Equal(t, received, channels*sendsPerChannel*receivedPerSend)
	require.Equal(t, dropped, channels*sendsPerChannel*droppedPerSend)
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
