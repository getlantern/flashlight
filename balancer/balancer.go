// Package balancer provides load balancing of network connections per
// different strategies.
package balancer

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/ema"
	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/ops"
)

const (
	dialAttempts = 3

	// reasonable but slightly larger than the timeout of all dialers
	initialTimeout = 1 * time.Minute
)

var (
	log = golog.LoggerFor("balancer")
)

// Opts are options for the balancer.
type Opts struct {
	// Dialers are the Dialers amongst which the Balancer will balance.
	Dialers []*Dialer

	// MinCheckInterval controls the minimum check interval when scheduling dialer
	// checks. Defaults to 10 seconds.
	MinCheckInterval time.Duration

	// MaxCheckInterval controls the maximum check interval when scheduling dialer
	// checks. Defaults to 1 minute.
	MaxCheckInterval time.Duration
}

// Balancer balances connections among multiple Dialers.
type Balancer struct {
	lastDialTime int64 // not used anymore, but makes sure we're aligned on 64bit boundary
	nextTimeout  *ema.EMA
	mu           sync.RWMutex
	dialers      sortedDialers
	trusted      sortedDialers
	closeCh      chan bool
	stopStatsCh  chan bool
	forceStatsCh chan bool
}

// New creates a new Balancer using the supplied Strategy and Dialers.
func New(opts *Opts) *Balancer {
	// a small alpha to gradually adjust timeout based on performance of all
	// dialers
	b := &Balancer{
		nextTimeout:  ema.NewDuration(initialTimeout, 0.2),
		closeCh:      make(chan bool),
		stopStatsCh:  make(chan bool, 1),
		forceStatsCh: make(chan bool, 1),
	}

	b.Reset(opts.Dialers...)
	ops.Go(b.printStats)
	ops.Go(b.run)
	return b
}

func (b *Balancer) dialersToCheck() []*dialer {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ds := make([]*dialer, 0, len(b.dialers))
	for _, d := range b.dialers {
		ds = append(ds, d)
	}
	return ds
}

// Reset closes existing dialers and replaces them with new ones.
func (b *Balancer) Reset(dialers ...*Dialer) {
	// TODO: track estimated latency and estimated bandwidth on our dialer so that
	// we can save and transfer accordingly.

	// TODO: if dialing fails, start rechecking dials with exponential backoff

	// TODO: report estimated bandwidth and estimated latency to borda sometimes
	log.Debug("Resetting")
	var dls sortedDialers
	var tdls sortedDialers

	b.mu.Lock()
	oldDialers := b.dialers
	for _, d := range dialers {
		dl := &dialer{Dialer: d}
		for _, od := range oldDialers {
			if d.Label == od.Label {
				// Existing dialer, keep stats
				log.Debugf("Keeping stats from old dialer")
				dl.consecSuccesses = atomic.LoadInt32(&od.consecSuccesses)
				dl.consecFailures = atomic.LoadInt32(&od.consecFailures)
				dl.stats = od.stats
				break
			}
		}
		dl.Start()
		dls = append(dls, dl)

		if dl.Trusted {
			tdls = append(tdls, dl)
		}
	}
	b.dialers = dls
	b.trusted = tdls
	b.mu.Unlock()
	for _, d := range oldDialers {
		d.Stop()
	}
}

// Dial dials (network, addr) using one of the currently active configured
// Dialers. The Dialer to choose depends on the Strategy when creating the
// balancer. Only Trusted Dialers are used to dial HTTP hosts.
//
// If a Dialer fails to connect, Dial will keep trying at most 3 times until it
// either manages to connect, or runs out of dialers in which case it returns an
// error.
func (b *Balancer) Dial(network, addr string) (net.Conn, error) {
	trustedOnly := false
	var lastDialer *dialer
	for i := 0; i < dialAttempts; i++ {
		d, pickErr := b.pickDialer(trustedOnly)
		if pickErr != nil {
			return nil, pickErr
		}
		if d == lastDialer {
			log.Debugf("Skip dialing %s://%s with same dailer %s", network, addr, d.Label)
			continue
		}
		lastDialer = d
		log.Tracef("Dialing %s://%s with %s", network, addr, d.Label)

		conn, err := b.dialWithTimeout(d, network, addr)
		if err != nil {
			log.Error(errors.New("Unable to dial via %v to %s://%s: %v on pass %v...continuing", d.Label, network, addr, err, i))
			continue
		}
		// Please leave this at Debug level, as it helps us understand performance
		// issues caused by a poor proxy being selected.
		log.Debugf("Successfully dialed via %v to %v://%v on pass %v", d.Label, network, addr, i)
		return conn, nil
	}
	return nil, fmt.Errorf("Still unable to dial %s://%s after %d attempts", network, addr, dialAttempts)
}

func (b *Balancer) dialWithTimeout(d *dialer, network, addr string) (net.Conn, error) {
	limit := b.nextTimeout.GetDuration()
	timer := time.NewTimer(limit)
	var conn net.Conn
	var err error
	// to synchronize access of conn and err between outer and inner goroutine
	chDone := make(chan bool)
	t := time.Now()
	ops.Go(func() {
		conn, err = d.dial(network, addr)
		if err == nil {
			newTimeout := b.nextTimeout.UpdateDuration(3 * time.Since(t))
			log.Tracef("Updated nextTimeout to %v", newTimeout)
		}
		chDone <- true
	})
	for {
		select {
		case _ = <-timer.C:
			// give current dialer a chance to return/fail and other dialers to
			// take part in.
			if d.ConsecSuccesses() > 0 {
				log.Debugf("Reset balancer dial timeout because dialer %s suddenly slows down", d.Label)
				b.nextTimeout.SetDuration(initialTimeout)
				timer.Reset(initialTimeout)
				continue
			}
			// clean up
			ops.Go(func() {
				_ = <-chDone
				if conn != nil {
					_ = conn.Close()
				}
			})
			return nil, errors.New("timeout").With("limit", limit)
		case _ = <-chDone:
			return conn, err
		}
	}
}

func (b *Balancer) run() {
	for {
		select {
		case <-b.closeCh:
			b.stopStatsCh <- true

			b.mu.Lock()
			oldDialers := b.dialers
			b.dialers = nil
			b.mu.Unlock()
			for _, d := range oldDialers {
				d.Stop()
			}
			// Make sure everything is actually cleaned up before the caller continues.
			b.closeCh <- true
			return
		}
	}
}

// Close closes this Balancer, stopping all background processing. You must call
// Close to avoid leaking goroutines.
func (b *Balancer) Close() {
	select {
	case b.closeCh <- true:
		// Submitted close request
		<-b.closeCh
	default:
		// already closing
	}
}

// printStats periodically prints out stats for all dialers
func (b *Balancer) printStats() {
	// TODO: print these less frequently
	t := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-b.stopStatsCh:
			return
		case <-b.forceStatsCh:
			b.doPrintStats()
		case <-t.C:
			b.doPrintStats()
		}
	}
}

func (b *Balancer) doPrintStats() {
	b.mu.RLock()
	defer b.mu.RUnlock()
	dialersCopy := make(sortedDialers, len(b.dialers))
	copy(dialersCopy, b.dialers)
	sort.Sort(dialersCopy)
	log.Debug("-------------------------- Dialer Stats -----------------------")
	for _, d := range dialersCopy {
		log.Debug(d.stats.String(d))
	}
	log.Debug("------------------------ End Dialer Stats ---------------------")
}

func (b *Balancer) forceStats() {
	select {
	case b.forceStatsCh <- true:
		// okay
	default:
		// already pending
	}
}

func (b *Balancer) pickDialer(trustedOnly bool) (*dialer, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	dialers := b.dialers
	if trustedOnly {
		dialers = b.trusted
	}
	if dialers.Len() == 0 {
		if trustedOnly {
			return nil, fmt.Errorf("No trusted dialers")
		}
		return nil, fmt.Errorf("No dialers")
	}
	sort.Sort(dialers)
	return dialers[0], nil
}

type sortedDialers []*dialer

func (d sortedDialers) Len() int { return len(d) }

func (d sortedDialers) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}

func (d sortedDialers) Less(i, j int) bool {
	// TODO: take into account availability/failure rate

	a, b := d[i], d[j]

	// Prefer the succeeding proxy
	aSucceeding, bSucceeding := a.Succeeding(), b.Succeeding()
	if aSucceeding && !bSucceeding {
		return true
	}
	if !aSucceeding && bSucceeding {
		return false
	}

	// while proxy's bandwidth is unknown, it should get traffic
	eba, ebb := a.EstBandwidth(), b.EstBandwidth()
	ebaKnown, ebbKnown := eba > 0, ebb > 0
	if !ebaKnown && ebbKnown {
		return true
	}
	if ebaKnown && !ebbKnown {
		return false
	}
	if !ebaKnown && !ebbKnown {
		// bandwidth is known for neither proxy, sort by label to keep sending
		// traffic to same proxy until we know bandwidth.
		return strings.Compare(a.Label, b.Label) < 0
	}

	// divide bandwidth by latency to determine how to sort
	ela, elb := a.EstLatency().Seconds(), b.EstLatency().Seconds()
	return float64(eba)/ela > float64(ebb)/elb
}
