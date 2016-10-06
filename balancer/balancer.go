// Package balancer provides load balancing of network connections per
// different strategies.
package balancer

import (
	"container/heap"
	"fmt"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/ops"
)

const (
	dialAttempts = 3

	// reasonable but slightly larger than the timeout of all dialers
	initialTimeout = 1 * time.Minute

	defaultMinCheckInterval = 10 * time.Second
	defaultMaxCheckInterval = 1 * time.Hour
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

	// CheckData returns data to be used by each dialer's Check function on a
	// given checking pass.
	CheckData func() interface{}

	// MinCheckInterval controls the minimum check interval when scheduling dialer
	// checks. Defaults to 10 seconds.
	MinCheckInterval time.Duration

	// MaxCheckInterval controls the maximum check interval when scheduling dialer
	// checks. Defaults to 1 minute.
	MaxCheckInterval time.Duration
}

// Balancer balances connections among multiple Dialers.
type Balancer struct {
	lastDialTime     int64 // not used anymore, but makes sure we're aligned on 64bit boundary
	nextTimeout      *emaDuration
	st               Strategy
	mu               sync.RWMutex
	dialers          dialerHeap
	trusted          dialerHeap
	checkData        func() interface{}
	minCheckInterval time.Duration
	maxCheckInterval time.Duration
	resetCheckCh     chan bool
	closeCh          chan bool
	stopStats        chan bool
}

// New creates a new Balancer using the supplied Strategy and Dialers.
func New(opts *Opts) *Balancer {
	// a small alpha to gradually adjust timeout based on performance of all
	// dialers
	b := &Balancer{
		st:               opts.Strategy,
		nextTimeout:      newEMADuration(initialTimeout, 0.2),
		checkData:        opts.CheckData,
		minCheckInterval: opts.MinCheckInterval,
		maxCheckInterval: opts.MaxCheckInterval,
		resetCheckCh:     make(chan bool, 1),
		closeCh:          make(chan bool, 1),
		stopStats:        make(chan bool, 0),
	}
	if b.checkData == nil {
		b.checkData = func() interface{} { return nil }
	}
	if b.minCheckInterval <= 0 {
		b.minCheckInterval = defaultMinCheckInterval
	}
	if b.maxCheckInterval <= 0 {
		b.maxCheckInterval = defaultMaxCheckInterval
	}
	b.Reset(opts.Dialers...)
	ops.Go(b.runChecks)
	ops.Go(b.printStats)
	return b
}

// Reset closes existing dialers and replaces them with new ones.
func (b *Balancer) Reset(dialers ...*Dialer) {
	var dls []*dialer
	var tdls []*dialer

	for _, d := range dialers {
		dl := &dialer{Dialer: d, forceRecheck: b.forceRecheck}
		dl.Start()
		dls = append(dls, dl)

		if dl.Trusted {
			tdls = append(tdls, dl)
		}
	}
	b.mu.Lock()
	oldDialers := b.dialers
	b.dialers = b.st(dls)
	b.trusted = b.st(tdls)
	heap.Init(&b.dialers)
	heap.Init(&b.trusted)
	b.mu.Unlock()
	for _, d := range oldDialers.dialers {
		d.Stop()
	}
	b.forceRecheck()
}

// OnRequest calls Dialer.OnRequest for every dialer in this balancer.
func (b *Balancer) OnRequest(req *http.Request) {
	b.mu.RLock()
	b.dialers.onRequest(req)
	b.mu.RUnlock()
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
	limit := b.nextTimeout.Get()
	timer := time.NewTimer(limit)
	var conn net.Conn
	var err error
	// to synchronize access of conn and err between outer and inner goroutine
	chDone := make(chan bool)
	t := time.Now()
	ops.Go(func() {
		conn, err = d.dial(network, addr)
		if err == nil {
			newTimeout := b.nextTimeout.UpdateWith(3 * time.Since(t))
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
				b.nextTimeout.Set(initialTimeout)
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

// Close closes this Balancer, stopping all background processing. You must call
// Close to avoid leaking goroutines.
func (b *Balancer) Close() {
	select {
	case b.closeCh <- true:
		// Submitted close request
	default:
		// already closing
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

// printStats periodically prints out stats for all dialers
func (b *Balancer) printStats() {
	t := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-b.stopStats:
			return
		case <-t.C:
			b.mu.RLock()
			sortedDialers := make(byLatency, len(b.dialers.dialers))
			copy(sortedDialers, b.dialers.dialers)
			b.mu.RUnlock()
			sort.Sort(sortedDialers)
			for _, d := range sortedDialers {
				log.Debug(d.stats.String(d))
			}
		}
	}
}

type dialerHeap struct {
	dialers  []*dialer
	lessFunc func(i, j int) bool
}

func (s *dialerHeap) Len() int { return len(s.dialers) }

func (s *dialerHeap) Swap(i, j int) {
	s.dialers[i], s.dialers[j] = s.dialers[j], s.dialers[i]
}

func (s *dialerHeap) Less(i, j int) bool {
	return s.lessFunc(i, j)
}

func (s *dialerHeap) Push(x interface{}) {
	s.dialers = append(s.dialers, x.(*dialer))
}

func (s *dialerHeap) Pop() interface{} {
	old := s.dialers
	n := len(old)
	x := old[n-1]
	s.dialers = old[0 : n-1]
	return x
}

func (s *dialerHeap) onRequest(req *http.Request) {
	for _, d := range s.dialers {
		if d.OnRequest != nil {
			d.OnRequest(req)
		}
	}
	return
}

type byLatency []*dialer

func (d byLatency) Len() int { return len(d) }

func (d byLatency) Swap(i, j int) { d[i], d[j] = d[j], d[i] }

func (d byLatency) Less(i, j int) bool {
	return d[i].EMALatency() < d[j].EMALatency()
}
