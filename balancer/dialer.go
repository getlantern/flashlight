package balancer

import (
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/getlantern/ema"
	"github.com/getlantern/flashlight/ops"
)

var (
	// ErrUpstream is an error that indicates there was a problem upstream of a
	// proxy. Such errors are not counted as failures but do allow failover to
	// other proxies.
	ErrUpstream = errors.New("Upstream error")
)

// Dialer captures the configuration for dialing arbitrary addresses.
type Dialer struct {
	// Label: optional label with which to tag this dialer for debug logging.
	Label string

	// DialFN: this function dials the given network, addr.
	DialFN func(network, addr string) (net.Conn, error)

	// OnClose: (optional) callback for when this dialer is stopped.
	OnClose func()

	// Host holds the host IP address or name for this dialer
	Host string

	// Determines whether a dialer can be trusted with unencrypted traffic.
	Trusted bool
}

type dialer struct {
	emaRTT              *ema.EMA
	emaPLR              *ema.EMA
	estimatedThroughput int64
	forceRecheck        func()

	*Dialer
	closeCh chan struct{}

	consecSuccesses int32
	consecFailures  int32

	stats *stats
}

const longDuration = 100000 * time.Hour

func (d *dialer) Start() {
	d.consecSuccesses = 1 // be optimistic
	if d.emaRTT == nil {
		// Assume okay RTT and 0 packet loss to start
		// use large alpha to reflect network changes quickly
		d.emaRTT = ema.NewDuration(250*time.Millisecond, 0.5)
		d.emaPLR = ema.New(0, 0.5)
		atomic.StoreInt64(&d.estimatedThroughput, mathisThroughput(d.emaRTT.GetDuration(), d.emaPLR.Get()))
	}
	if d.stats == nil {
		d.stats = &stats{}
	}
	d.closeCh = make(chan struct{}, 1)

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

func (d *dialer) EstimatedThroughput() int64 {
	return atomic.LoadInt64(&d.estimatedThroughput)
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
		if err != ErrUpstream {
			d.markFailure()
		}
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
	d.doMarkFailure()
	d.forceRecheck()
}

func (d *dialer) doMarkFailure() {
	atomic.AddInt64(&d.stats.attempts, 1)
	atomic.AddInt64(&d.stats.failures, 1)
	atomic.StoreInt32(&d.consecSuccesses, 0)
	newCF := atomic.AddInt32(&d.consecFailures, 1)
	log.Tracef("Dialer %s consecutive failures: %d -> %d", d.Label, newCF-1, newCF)
}
