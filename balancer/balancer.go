// Package balancer provides load balancing of network connections per
// different strategies.
package balancer

import (
	"container/heap"
	"fmt"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/ema"
	"github.com/getlantern/errors"
	"github.com/getlantern/go-ping"
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
	// Strategy is the strategy to be used for load balancing between the dialers.
	Strategy Strategy

	// Dialers are the Dialers amongst which the Balancer will balance.
	Dialers []*Dialer

	// FastCheckInterval controls the rate at which the top dialer is checked.
	// Defaults to 15 seconds.
	FastCheckInterval time.Duration

	// SlowCheckInterval controls the rate at which all dialers are checked.
	// Defaults to 5 minutes.
	SlowCheckInterval time.Duration
}

// Balancer balances connections among multiple Dialers.
type Balancer struct {
	lastDialTime   int64 // not used anymore, but makes sure we're aligned on 64bit boundary
	nextTimeout    *ema.EMA
	st             Strategy
	mu             sync.RWMutex
	dialers        dialerHeap
	trusted        dialerHeap
	closeCh        chan bool
	resetCheckCh   chan bool
	stopStatsCh    chan bool
	forceStatsCh   chan bool
	checkerCloseCh chan bool
}

// New creates a new Balancer using the supplied Strategy and Dialers.
func New(opts *Opts) *Balancer {
	resetCheckCh := make(chan bool, 1)
	checkerCloseCh := make(chan bool)
	// a small alpha to gradually adjust timeout based on performance of all
	// dialers
	b := &Balancer{
		st:             opts.Strategy,
		nextTimeout:    ema.NewDuration(initialTimeout, 0.2),
		closeCh:        make(chan bool),
		stopStatsCh:    make(chan bool, 1),
		forceStatsCh:   make(chan bool, 1),
		resetCheckCh:   resetCheckCh,
		checkerCloseCh: checkerCloseCh,
	}

	pinger, err := ping.NewPinger(false)
	if err != nil {
		panic(fmt.Sprintf("Unable to create pinger: %v", err))
	}

	checker := &checker{
		b:                 b,
		pinger:            pinger,
		fastCheckInterval: opts.FastCheckInterval,
		slowCheckInterval: opts.SlowCheckInterval,
		resetCheckCh:      resetCheckCh,
		closeCh:           checkerCloseCh,
	}

	if checker.fastCheckInterval <= 0 {
		checker.fastCheckInterval = defaultFastCheckInterval
	}
	if checker.slowCheckInterval <= 0 {
		checker.slowCheckInterval = defaultSlowCheckInterval
	}

	b.Reset(opts.Dialers...)
	ops.Go(checker.runChecks)
	ops.Go(b.printStats)
	ops.Go(b.run)
	return b
}

// Reset closes existing dialers and replaces them with new ones.
func (b *Balancer) Reset(dialers ...*Dialer) {
	log.Debug("Resetting")
	var dls []*dialer
	var tdls []*dialer

	b.mu.Lock()
	oldDialers := b.dialers.dialers
	for _, d := range dialers {
		dl := &dialer{Dialer: d, forceRecheck: b.forceRecheck}
		for _, od := range oldDialers {
			if d.Label == od.Label {
				// Existing dialer, keep stats
				dl.consecSuccesses = atomic.LoadInt32(&od.consecSuccesses)
				dl.consecFailures = atomic.LoadInt32(&od.consecFailures)
				dl.estimatedThroughput = atomic.LoadInt64(&od.estimatedThroughput)
				dl.emaRTT = od.emaRTT
				dl.emaPLR = od.emaPLR
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
	b.dialers = b.st(dls)
	b.trusted = b.st(tdls)
	heap.Init(&b.dialers)
	heap.Init(&b.trusted)
	b.mu.Unlock()
	for _, d := range oldDialers {
		d.Stop()
	}
	b.forceRecheck()
}

func (b *Balancer) forceRecheck() {
	select {
	case b.resetCheckCh <- true:
		log.Debug("Forced recheck")
	default:
		// Pending recheck, ignore subsequent request
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
	_, port, _ := net.SplitHostPort(addr)
	// We try to identify HTTP traffic (as opposed to HTTPS) by port and only
	// send HTTP traffic to dialers marked as trusted.
	if port == "" || port == "80" || port == "8080" {
		trustedOnly = true
	}

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

			b.checkerCloseCh <- true
			// Make sure it actually finishes.
			<-b.checkerCloseCh

			b.mu.Lock()
			oldDialers := b.dialers
			b.dialers.dialers = nil
			b.mu.Unlock()
			for _, d := range oldDialers.dialers {
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
	t := time.NewTicker(30 * time.Second)
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
	sortedDialers := b.sortedDialers()
	log.Debug("-------------------------- Dialer Stats -----------------------")
	for _, d := range sortedDialers {
		log.Debug(d.stats.String(d))
	}
	log.Debug("------------------------ End Dialer Stats ---------------------")
}

func (b *Balancer) sortedDialers() []*dialer {
	b.mu.RLock()
	dialersCopy := make([]*dialer, len(b.dialers.dialers))
	copy(dialersCopy, b.dialers.dialers)
	b.mu.RUnlock()
	sortedDialers := b.st(dialersCopy)
	sort.Sort(sortedDialers)
	return sortedDialers.dialers
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
	dialers := &b.dialers
	if trustedOnly {
		dialers = &b.trusted
	}
	if dialers.Len() == 0 {
		if trustedOnly {
			return nil, fmt.Errorf("No trusted dialers")
		}
		return nil, fmt.Errorf("No dialers")
	}
	// heap will re-adjust based on new metrics
	d := heap.Pop(dialers).(*dialer)
	heap.Push(dialers, d)
	return d, nil
}

type dialerHeap struct {
	dialers  []*dialer
	lessFunc func(i, j int) bool
}

func (s dialerHeap) Len() int { return len(s.dialers) }

func (s dialerHeap) Swap(i, j int) {
	s.dialers[i], s.dialers[j] = s.dialers[j], s.dialers[i]
}

func (s dialerHeap) Less(i, j int) bool {
	return s.lessFunc(i, j)
}

func (s dialerHeap) Push(x interface{}) {
	s.dialers = append(s.dialers, x.(*dialer))
}

func (s dialerHeap) Pop() interface{} {
	old := s.dialers
	n := len(old)
	x := old[n-1]
	s.dialers = old[0 : n-1]
	return x
}
