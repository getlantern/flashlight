package balancer

import (
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/getlantern/flashlight/ops"
)

const (
	minCheckInterval = 10 * time.Second
	maxCheckInterval = 15 * time.Minute
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

	// Determines whether a dialer can be trusted with unencrypted traffic.
	Trusted bool

	// EstLatency() provides a latency estimate
	EstLatency func() time.Duration

	// EstBandwidth() provides the estimated bandwidth in Mbps
	EstBandwidth func() float64

	// ForceRecheckCh is a channel on which requests to force rechecking are
	// received.
	ForceRecheckCh chan bool
}

type dialer struct {
	lastCheckSucceeded bool

	*Dialer
	closeCh chan struct{}

	consecSuccesses int32
	consecFailures  int32

	stats *stats
}

const longDuration = 100000 * time.Hour

func (d *dialer) Start() {
	d.consecSuccesses = 1 // be optimistic
	if d.stats == nil {
		d.stats = &stats{}
	}
	d.closeCh = make(chan struct{}, 1)

	// Periodically check our connectivity.
	// With a 15 minute period, Lantern running 8 hours a day for 30 days and 148
	// bytes for a TCP connection setup and teardown, this check will consume
	// approximately 138 KB per month per proxy.
	checkInterval := maxCheckInterval
	timer := time.NewTimer(checkInterval)

	ops.Go(func() {
		for {
			select {
			case <-timer.C:
				log.Debugf("Checking %v", d.Label)
				conn, err := d.dial("tcp", "www.getlantern.org:80")
				if err == nil {
					d.markSuccess()
					conn.Close()
					// On success, don't bother rechecking anytime soon
					checkInterval = maxCheckInterval
				} else {
					d.markFailure()
					// Exponentially back off while we're still failing
					checkInterval *= 2
					if checkInterval > maxCheckInterval {
						checkInterval = maxCheckInterval
					}
				}
				timer.Reset(checkInterval)
			case <-d.ForceRecheckCh:
				log.Debugf("Forcing recheck for %v", d.Label)
				checkInterval := minCheckInterval
				timer.Reset(checkInterval)
			case <-d.closeCh:
				log.Tracef("Dialer %v stopped", d.Label)
				if d.OnClose != nil {
					d.OnClose()
				}
				timer.Stop()
				return
			}
		}
	})
}

func (d *dialer) Stop() {
	log.Tracef("Stopping dialer %s", d.Label)
	d.closeCh <- struct{}{}
}

func (d *dialer) ConsecSuccesses() int32 {
	return atomic.LoadInt32(&d.consecSuccesses)
}

func (d *dialer) ConsecFailures() int32 {
	return atomic.LoadInt32(&d.consecFailures)
}

func (d *dialer) Succeeding() bool {
	return d.ConsecSuccesses()-d.ConsecFailures() > 0
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
	if d.doMarkFailure() == 1 {
		// On new failure, force recheck
		d.forceRecheck()
	}
}

func (d *dialer) doMarkFailure() int32 {
	atomic.AddInt64(&d.stats.attempts, 1)
	atomic.AddInt64(&d.stats.failures, 1)
	atomic.StoreInt32(&d.consecSuccesses, 0)
	newCF := atomic.AddInt32(&d.consecFailures, 1)
	log.Tracef("Dialer %s consecutive failures: %d -> %d", d.Label, newCF-1, newCF)
	return newCF
}

func (d *dialer) forceRecheck() {
	select {
	case d.ForceRecheckCh <- true:
		// requested
	default:
		// recheck already requested, ignore
	}
}
