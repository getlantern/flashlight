package balancer

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/mockconn"
)

var (
	rttMultiplier = time.Duration(1)
)

type testDialer struct {
	name               string
	baseRTT            time.Duration
	rtt                time.Duration
	bandwidth          float64
	untrusted          int32
	failingUpstream    bool
	muSuccessRate      sync.RWMutex
	successRate        float64
	attempts           int64
	successes          int64
	failures           int64
	stopped            int32
	connectivityChecks int32
	remainingFailures  int
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

func (d *testDialer) Location() (string, string, string) {
	return "", "", ""
}

func (d *testDialer) Protocol() string {
	return "https"
}

func (d *testDialer) Addr() string {
	return ""
}

func (d *testDialer) Trusted() bool {
	return atomic.LoadInt32(&d.untrusted) == 0
}

func (d *testDialer) NumPreconnecting() int {
	return 0
}

func (d *testDialer) NumPreconnected() int {
	return 0
}

func (d *testDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, bool, error) {
	var conn net.Conn
	var err error
	if d.remainingFailures > 0 {
		err = fmt.Errorf("Failing intentionally")
		d.remainingFailures--
	} else if !d.Succeeding() {
		err = fmt.Errorf("Not succeeding")
	} else if d.failingUpstream {
		err = fmt.Errorf("Failing upstream")
	} else {
		time.Sleep(d.rtt)
		if network != "" {
			var d net.Dialer
			conn, err = d.DialContext(ctx, network, addr)
		} else {
			var buf bytes.Buffer
			conn = mockconn.New(&buf, strings.NewReader(""))
		}
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

func (d *testDialer) setSuccessRate(rate float64) {
	d.muSuccessRate.Lock()
	defer d.muSuccessRate.Unlock()
	d.successRate = rate
}

func (d *testDialer) EstSuccessRate() float64 {
	d.muSuccessRate.RLock()
	defer d.muSuccessRate.RUnlock()
	return d.successRate
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
	return d.EstSuccessRate() > 0.9
}

func (d *testDialer) recalcRTT() {
	if d.baseRTT != 0 {
		d.rtt = d.baseRTT * rttMultiplier
	}
}

func (d *testDialer) connectivityChecksSinceLast() int {
	return int(atomic.SwapInt32(&d.connectivityChecks, 0))
}

func (d *testDialer) DataSent() uint64 {
	return 0
}

func (d *testDialer) DataRecv() uint64 {
	return 0
}

func (d *testDialer) Probe(forPerformance bool) bool {
	d.recalcRTT()
	time.Sleep(d.rtt)
	atomic.AddInt32(&d.connectivityChecks, 1)
	return true
}

func (d *testDialer) ProbeStats() (successes uint64, successKBs uint64, failures uint64, failedKBs uint64) {
	return 0, 0, 0, 0
}

func (d *testDialer) Stop() {
	atomic.StoreInt32(&d.stopped, 1)
}

func (d *testDialer) Ping() {}
