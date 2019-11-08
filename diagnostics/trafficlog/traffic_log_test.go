package trafficlog

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Some tests in this package require elevated permissions and are thus disabled by default. Set
// runElevated to true to run these tests. Alternatively, use the flag from the command line.
const runElevated = false

var runElevatedFlag = flag.Bool(
	"force-diagnostics-tests",
	runElevated,
	"run tests in github.com/getlantern/flashlight/diagnostics requiring elevated permissions",
)

func init() {
	flag.Parse()
}

func TestTrafficLog(t *testing.T) {
	if !*runElevatedFlag {
		t.SkipNow()
	}

	const (
		captureAddresses     = 10
		serverResponseString = "TestTrafficLog test server response"

		// Make the buffers large enough that we will not lose any packets.
		captureBufferSize, saveBufferSize = 1024 * 1024, 1024 * 1024

		// The time we allow for capture to start or take place.
		captureWaitTime = 200 * time.Millisecond
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

	tl := New(captureBufferSize, saveBufferSize, &Options{MTULimit: MTULimitNone})
	require.NoError(t, tl.UpdateAddresses(addresses))
	defer tl.Close()

	go func() {
		for err := range tl.Errors() {
			t.Fatal(err)
		}
	}()

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
		updateInterval    = time.Hour // doesn't matter for this test
	)

	st := newStatsTracker(updateInterval)
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
	st.close()

	var received, dropped uint64
	for stats := range st.output {
		received, dropped = stats.Received, stats.Dropped
	}
	require.Equal(t, received, channels*sendsPerChannel*receivedPerSend)
	require.Equal(t, dropped, channels*sendsPerChannel*droppedPerSend)
}

// TestPacketOverhead checks that the packet overhead value is still accurate.
func TestPacketOverhead(t *testing.T) {
	if !*runElevatedFlag {
		t.SkipNow()
	}

	type oppOutput struct {
		MeanPacketOverhead        float64
		OverheadStandardDeviation float64
	}

	cmdOutput, err := exec.Command("go", "run", "./opp/main.go").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatal(string(exitErr.Stderr))
		} else {
			t.Fatal(err)
		}
	}

	var parsedOutput oppOutput
	require.NoError(t, json.Unmarshal(cmdOutput, &parsedOutput))
	require.Less(
		t, parsedOutput.OverheadStandardDeviation/parsedOutput.MeanPacketOverhead, 0.1,
		"Standard deviation was too large for an accurate test. Mean overhead: %f; standard deviation: %f",
		parsedOutput.MeanPacketOverhead, parsedOutput.OverheadStandardDeviation,
	)
	require.Less(
		t, math.Abs(parsedOutput.MeanPacketOverhead-float64(overheadPerPacket))/parsedOutput.MeanPacketOverhead, 0.1,
		"Overhead-per-packet is not accurate. Actual: %f; configured: %d",
		parsedOutput.MeanPacketOverhead, overheadPerPacket,
	)
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
