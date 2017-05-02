package balancer

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

type testDialer struct {
	name      string
	latency   time.Duration
	bandwidth float64
	untrusted bool
	failing   bool
	attempts  int64
	successes int64
	failures  int64
	stopped   bool
}

// Name returns the name for this Dialer
func (d *testDialer) Name() string {
	return d.name
}

func (d *testDialer) Label() string {
	return d.name
}

func (d *testDialer) JustifiedLabel() string {
	return d.name
}

func (d *testDialer) Addr() string {
	return ""
}

func (d *testDialer) Trusted() bool {
	return !d.untrusted
}

func (d *testDialer) Dial(network, addr string) (net.Conn, error) {
	var conn net.Conn
	var err error
	if !d.Succeeding() {
		err = fmt.Errorf("Failing intentionally")
	} else if network != "" {
		conn, err = net.DialTimeout(network, addr, 250*time.Millisecond)
	}
	atomic.AddInt64(&d.attempts, 1)
	if err == nil {
		atomic.AddInt64(&d.successes, 1)
	} else {
		atomic.AddInt64(&d.failures, 1)
	}
	return conn, err
}

func (d *testDialer) attemptsSinceLast() int {
	attempts := d.Attempts()
	atomic.StoreInt64(&d.attempts, 0)
	return int(attempts)
}

func (d *testDialer) EstLatency() time.Duration {
	return d.latency
}

func (d *testDialer) EstBandwidth() float64 {
	return d.bandwidth
}

func (d *testDialer) Attempts() int64 {
	return atomic.LoadInt64(&d.attempts)
}

func (d *testDialer) Successes() int64 {
	return atomic.LoadInt64(&d.successes)
}

func (d *testDialer) ConsecSuccesses() int64 {
	return 0
}

func (d *testDialer) Failures() int64 {
	return atomic.LoadInt64(&d.failures)
}

func (d *testDialer) ConsecFailures() int64 {
	return 0
}

func (d *testDialer) Succeeding() bool {
	return !d.failing
}

func (d *testDialer) CheckConnectivity() bool {
	return true
}

func (d *testDialer) ProbePerformance() {
}

func (d *testDialer) Stop() {
	d.stopped = true
}
