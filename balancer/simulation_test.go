package balancer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBalancerSimulation(t *testing.T) {
	oldRecheckInterval := recheckInterval
	recheckInterval = 1 * time.Millisecond
	defer func() {
		recheckInterval = oldRecheckInterval
	}()

	a := &testDialer{
		name:        "a",
		baseLatency: 4 * time.Second,
		latency:     4 * time.Second,
		bandwidth:   0,
	}
	b := &testDialer{
		name:        "b",
		baseLatency: 2 * time.Second,
		latency:     2 * time.Second,
		bandwidth:   0,
	}
	c := &testDialer{
		name:        "c",
		baseLatency: 1 * time.Second,
		latency:     1 * time.Second,
		bandwidth:   0,
	}

	// initialize Balancer
	bal := &Balancer{}
	bal.Reset(a, b, c)
	assertDialerOrder("dialers with unknown bandwidth should sort by name", t, bal, a, b, c)

	// make bandwidth known for one dialer
	a.bandwidth = 20000
	assertDialerOrder("sort order should remain unchanged before calling eval", t, bal, a, b, c)
	bal.evalDialers()
	assertDialerOrder("dialers with unknown bandwidth should sort before those with known bandwidth", t, bal, b, c, a)
	assertChecksSinceLast(t, bal, 0)

	// fill out bandwidth for all dialers
	b.bandwidth = 5000
	c.bandwidth = 1250
	bal.evalDialers()
	assertDialerOrder("dialers should sort by combination of bandwidth and latency", t, bal, a, b, c)

	// dramatically increase latency across the board
	latencyMultiplier = 10
	a.recalcLatency()
	assertDialerOrder("sort order should remain the same even after dramatically increased latency across the board", t, bal, a, b, c)
	bal.evalDialers()
	assertDialerOrder("sort order should remain the same even after generally increased latencies", t, bal, a, b, c)
	assertChecksSinceLast(t, bal, connectivityRechecks)

	// dramatically drop latency across the board
	latencyMultiplier = 1
	a.recalcLatency()
	assertDialerOrder("sort order should remain the same even after dramatically decreased latency across the board", t, bal, a, b, c)
	bal.evalDialers()
	assertDialerOrder("sort order should remain the same even after generally decreased latencies", t, bal, a, b, c)
	assertChecksSinceLast(t, bal, 0)

	// dramatically increase latency for top dialer
	a.baseLatency *= 100
	a.recalcLatency()
	bal.evalDialers()
	assertDialerOrder("top dialer should have changed after latency jump", t, bal, b, c, a)
	assertChecksSinceLast(t, bal, connectivityRechecks)

	// recover latency for top dialer
	a.baseLatency /= 100
	a.recalcLatency()
	bal.evalDialers()
	assertDialerOrder("top dialer should have changed after latency decrease", t, bal, a, b, c)
	assertChecksSinceLast(t, bal, connectivityRechecks)
}

func assertDialerOrder(scenario string, t *testing.T, bal *Balancer, expectedDialers ...Dialer) {
	expected := make([]string, 0, len(expectedDialers))
	for _, dialer := range expectedDialers {
		expected = append(expected, dialer.Name())
	}

	dialers := bal.copyOfDialers()
	order := make([]string, 0, len(dialers))
	for _, dialer := range dialers {
		order = append(order, dialer.Name())
	}
	assert.EqualValues(t, expected, order, scenario)
}

func assertChecksSinceLast(t *testing.T, bal *Balancer, attempts int) {
	dialers := bal.copyOfDialers()
	for _, dialer := range dialers {
		assert.Equal(t, attempts, dialer.(*testDialer).connectivityChecksSinceLast(), "Dialer %v had the wrong number of connectivity checks since last time", dialer.Name())
	}
}
