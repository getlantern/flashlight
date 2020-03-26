package diagnostics

import (
	"flag"
	"net"
	"testing"

	"github.com/getlantern/flashlight/balancer"
	"github.com/stretchr/testify/require"
)

// Some tests in this package require elevated permissions and are thus disabled by default. Set
// runElevated to true to run these tests. Alternatively, use the flag from the command line
// (preceded by the -args flag: https://golang.org/pkg/cmd/go/internal/test/#pkg-variables).
const runElevated = false

var runElevatedFlag = flag.Bool(
	"force-diagnostics-tests",
	runElevated,
	"run tests in github.com/getlantern/flashlight/diagnostics requiring elevated permissions",
)

func TestRun(t *testing.T) {
	if !*runElevatedFlag {
		t.SkipNow()
	}
	forcePing = true
	defer func() { forcePing = false }()

	report := Run([]balancer.Dialer{
		// The port does not matter, but its presence is expected.
		testDialer{name: "localhost", addr: "127.0.0.1:999"},
	})

	_, ok := report.PingProxies["localhost"]
	require.True(t, ok)

	for name, pingResult := range report.PingProxies {
		if pingResult.Error != nil {
			t.Fatalf("error running ping for %s: %v", name, *pingResult.Error)
		}
		require.Equal(t, net.ParseIP("127.0.0.1"), pingResult.Stats.IPAddr.IP)
		require.Equal(t, float64(0), pingResult.Stats.PacketLoss)
		require.Equal(t, pingCount, pingResult.Stats.PacketsSent)
	}
}

type testDialer struct {
	balancer.Dialer
	name, addr string
}

func (td testDialer) Name() string { return td.name }
func (td testDialer) Addr() string { return td.addr }
