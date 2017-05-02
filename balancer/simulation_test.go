package balancer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBalancerSimulation(t *testing.T) {
	a := &testDialer{
		name:      "a",
		latency:   4 * time.Second,
		bandwidth: 0,
	}
	b := &testDialer{
		name:      "b",
		latency:   2 * time.Second,
		bandwidth: 0,
	}
	c := &testDialer{
		name:      "c",
		latency:   1 * time.Second,
		bandwidth: 0,
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
	assertAttemptsSinceLast(t, bal, 0)

	// fill out bandwidth for all dialers
	b.bandwidth = 5000
	c.bandwidth = 1250
	bal.evalDialers()
	assertDialerOrder("dialers should sort by combination of bandwidth and latency", t, bal, a, b, c)
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

func assertAttemptsSinceLast(t *testing.T, bal *Balancer, attempts int) {
	dialers := bal.copyOfDialers()
	for _, dialer := range dialers {
		assert.Equal(t, attempts, dialer.(*testDialer).attemptsSinceLast(), "Dialer %v had the wrong number of attempts since last check", dialer.Name())
	}
}
