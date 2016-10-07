package balancer

import (
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/getlantern/ops"
)

// Dialer captures the configuration for dialing arbitrary addresses.
type Dialer struct {
	// Label: optional label with which to tag this dialer for debug logging.
	Label string

	// DialFN: this function dials the given network, addr.
	DialFN func(network, addr string) (net.Conn, error)

	// OnClose: (optional) callback for when this dialer is stopped.
	OnClose func()

	// Check: - a function that's used to periodically test reachibility metrics.
	//
	// It should return true for a successful check. It should also return a
	// time.Duration measuring latency via this dialer. How "latency" is measured
	// is up to the check function. Dialers with the lowest latencies are
	// prioritized over those with higher latencies. Latencies for failed checks
	// are ignored.
	//
	// Checks are performed immediately at startup and then periodically
	// thereafter, with an exponential back-off capped at MaxCheckInterval. If the
	// checks fail for some reason, the exponential cascade will reset to the
	// MinCheckInterval and start backing off again.
	//
	// checkData is data from the balancer's CheckData() function.
	Check func(checkData interface{}, onFailure func(string)) (bool, time.Duration)

	// Determines whether a dialer can be trusted with unencrypted traffic.
	Trusted bool

	// Modifies any HTTP requests made using connections from this dialer.
	OnRequest func(req *http.Request)
}

type dialer struct {
	emaLatency         *emaDuration
	lastCheckSucceeded bool
	forceRecheck       func()

	*Dialer
	closeCh chan struct{}

	consecSuccesses int32
	consecFailures  int32

	stats *stats
}

const longDuration = 100000 * time.Hour

func (d *dialer) Start() {
	d.consecSuccesses = 1 // be optimistic
	// assuming all dialers super fast initially
	// use large alpha to reflect network changes quickly
	d.emaLatency = newEMADuration(0, 0.5)
	d.closeCh = make(chan struct{}, 1)
	d.stats = &stats{}

	ops.Go(func() {
		<-d.closeCh
		log.Tracef("Dialer %s stopped", d.Label)
		if d.OnClose != nil {
			d.OnClose()
		}
	})
}

func (d *dialer) Stop() {
	log.Tracef("Stopping dialer %s", d.Label)
	d.closeCh <- struct{}{}
}

func (d *dialer) EMALatency() int64 {
	return d.emaLatency.GetInt64()
}

func (d *dialer) ConsecSuccesses() int32 {
	return atomic.LoadInt32(&d.consecSuccesses)
}

func (d *dialer) ConsecFailures() int32 {
	return atomic.LoadInt32(&d.consecFailures)
}

func (d *dialer) dial(network, addr string) (net.Conn, error) {
	conn, err := d.DialFN(network, addr)
	if err != nil {
		d.markFailure()
	} else {
		d.markSuccess()
	}
	return conn, err
}

func (d *dialer) markSuccess() {
	atomic.AddInt64(&d.stats.attempts, 1)
	atomic.AddInt64(&d.stats.successes, 1)
	newCS := atomic.AddInt32(&d.consecSuccesses, 1)
	log.Tracef("Dialer %s consecutive successes: %d -> %d", d.Label, newCS-1, newCS)
	// only when state is changing
	if newCS <= 2 {
		atomic.StoreInt32(&d.consecFailures, 0)
	}
}

func (d *dialer) markFailure() {
	atomic.AddInt64(&d.stats.attempts, 1)
	atomic.AddInt64(&d.stats.failures, 1)
	newCF := atomic.AddInt32(&d.consecFailures, 1)
	d.forceRecheck()
	log.Tracef("Dialer %s consecutive failures: %d -> %d", d.Label, newCF-1, newCF)
}
