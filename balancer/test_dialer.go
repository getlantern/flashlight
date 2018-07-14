package balancer

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

var (
	rttMultiplier = time.Duration(1)
)

type testDialer struct {
	name               string
	baseRTT            time.Duration
	rtt                time.Duration
	bandwidth          float64
	untrusted          bool
	remainingFailures  int
	failingUpstream    bool
	attempts           int64
	successes          int64
	failures           int64
	stopped            int32
	connectivityChecks int
}

func start(d *testDialer) *testDialer {
	return d
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

func (d *testDialer) Protocol() string {
	return "https"
}

func (d *testDialer) Addr() string {
	return ""
}

func (d *testDialer) Trusted() bool {
	return !d.untrusted
}

func (d *testDialer) Preconnect() {
}

func (d *testDialer) NumPreconnecting() int {
	return 0
}

func (d *testDialer) NumPreconnected() int {
	return 1
}

func (d *testDialer) Preconnected() ProxyConnection {
	return d
}

func (d *testDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, bool, error) {
	var conn net.Conn
	var err error
	if !d.Succeeding() {
		err = fmt.Errorf("Failing intentionally")
		d.remainingFailures -= 1
	} else if d.failingUpstream {
		err = fmt.Errorf("Failing upstream")
	} else if !d.Succeeding() {

	} else if network != "" {
		var d net.Dialer
		conn, err = d.DialContext(ctx, network, addr)
	}
	atomic.AddInt64(&d.attempts, 1)
	if err == nil {
		atomic.AddInt64(&d.successes, 1)
	} else if !d.failingUpstream {
		atomic.AddInt64(&d.failures, 1)
	}
	return conn, d.failingUpstream, err
}

func (d *testDialer) MarkFailure() {
	atomic.AddInt64(&d.failures, 1)
}

func (d *testDialer) EstRTT() time.Duration {
	return d.rtt
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
	return d.remainingFailures == 0
}

func (d *testDialer) ForceRedial() {
}

func (d *testDialer) recalcRTT() {
	if d.baseRTT != 0 {
		d.rtt = d.baseRTT * rttMultiplier
	}
}

func (d *testDialer) connectivityChecksSinceLast() int {
	result := d.connectivityChecks
	d.connectivityChecks = 0
	return result
}

func (d *testDialer) Probe(forPerformance bool) bool {
	d.recalcRTT()
	d.connectivityChecks++
	return true
}

func (d *testDialer) Stop() {
	atomic.StoreInt32(&d.stopped, 1)
}
