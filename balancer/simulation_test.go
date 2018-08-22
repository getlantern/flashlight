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
		baseRTT:     4 * time.Second,
		rtt:         4 * time.Second,
		bandwidth:   0,
		successRate: 1,
	}
	b := &testDialer{
		name:        "b",
		baseRTT:     2 * time.Second,
		rtt:         2 * time.Second,
		bandwidth:   0,
		successRate: 1,
	}
	c := &testDialer{
		name:        "c",
		baseRTT:     1 * time.Second,
		rtt:         1 * time.Second,
		bandwidth:   0,
		successRate: 1,
	}

	// initialize Balancer
	bal := &Balancer{
		closeCh: make(chan struct{}),
	}
	bal.Reset([]Dialer{a, b, c})
	defer bal.Close()
	assertDialerOrder("dialers with unknown bandwidth should sort by name", t, bal, a, b, c)

	bal.evalDialers()
	// make bandwidth known for one dialer
	c.bandwidth = 20000
	assertDialerOrder("sort order should remain unchanged before calling eval", t, bal, a, b, c)
	bal.evalDialers()
	assertDialerOrder("dialers with known bandwidth should sort before those with unknown bandwidth", t, bal, c, a, b)
	assertChecksSinceLast(t, bal, 0)

	// fill out bandwidth for all dialers
	b.bandwidth = 5000
	a.bandwidth = 1250
	bal.evalDialers()
	assertDialerOrder("dialers should sort by combination of bandwidth and RTT if RTT is comparable", t, bal, c, b, a)

	// dramatically increase RTT across the board
	rttMultiplier = 10
	a.recalcRTT()
	assertDialerOrder("sort order should remain the same even after dramatically increased RTT across the board", t, bal, c, b, a)
	bal.evalDialers()
	assertDialerOrder("sort order should remain the same even after generally increased latencies", t, bal, c, b, a)

	// dramatically drop RTT across the board
	rttMultiplier = 1
	a.recalcRTT()
	assertDialerOrder("sort order should remain the same even after dramatically decreased RTT across the board", t, bal, c, b, a)
	bal.evalDialers()
	assertDialerOrder("sort order should remain the same even after generally decreased latencies", t, bal, c, b, a)
	assertChecksSinceLast(t, bal, 0)

	// dramatically increase RTT for top dialer
	c.baseRTT *= 100
	c.recalcRTT()
	bal.evalDialers()
	assertDialerOrder("top dialer should have changed after RTT jump", t, bal, b, a, c)

	// recover RTT for top dialer
	a.baseRTT /= 100
	a.recalcRTT()
	bal.evalDialers()
	assertDialerOrder("top dialer should have changed after RTT decrease", t, bal, a, b, c)
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
