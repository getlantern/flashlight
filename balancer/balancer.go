// Package balancer provides load balancing of network connections per
// different strategies.
package balancer

import (
	"fmt"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
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

// Dialer provides the ability to dial a proxy and obtain information needed to
// effectively load balance between dialers.
type Dialer interface {
	// Name returns the name for this Dialer
	Name() string

	// Label() returns a label for this Dialer (includes Name plus more).
	Label() string

	// JustifiedLabel() is like Label() but with elements justified for line-by
	// -line display.
	JustifiedLabel() string

	// Addr returns the address for this Dialer
	Addr() string

	// Trusted() indicates whether or not this dialer is trusted
	Trusted() bool

	// Dial with this dialer
	Dial(network, addr string) (net.Conn, error)

	// EstLatency() provides a latency estimate
	EstLatency() time.Duration

	// EstBandwidth() provides the estimated bandwidth in Mbps
	EstBandwidth() float64

	// Attempts returns the total number of dial attempts
	Attempts() int64

	// Successes returns the total number of dial successes
	Successes() int64

	// ConsecSuccesses returns the number of consecutive dial successes
	ConsecSuccesses() int64

	// Failures returns the total number of dial failures
	Failures() int64

	// ConsecFailures returns the number of consecutive dial failures
	ConsecFailures() int64

	// Succeeding indicates whether or not this dialer is currently good to use
	Succeeding() bool

	// ProbePerformance forces the dialer to actively probe to try to better
	// understand its performance.
	ProbePerformance()

	// Stop stops background processing for this Dialer.
	Stop()
}

// Balancer balances connections among multiple Dialers.
type Balancer struct {
	lastDialTime   int64 // not used anymore, but makes sure we're aligned on 64bit boundary
	nextTimeout    *ema.EMA
	mu             sync.RWMutex
	dialers        SortedDialers
	trusted        SortedDialers
	closeCh        chan bool
	stopStatsCh    chan bool
	forceStatsCh   chan bool
	onActiveDialer chan Dialer
}

// New creates a new Balancer using the supplied Dialers.
func New(dialers ...Dialer) *Balancer {
	// a small alpha to gradually adjust timeout based on performance of all
	// dialers
	b := &Balancer{
		nextTimeout:    ema.NewDuration(initialTimeout, 0.2),
		closeCh:        make(chan bool),
		stopStatsCh:    make(chan bool, 1),
		forceStatsCh:   make(chan bool, 1),
		onActiveDialer: make(chan Dialer, 1),
	}

	b.Reset(dialers...)
	ops.Go(b.printStats)
	ops.Go(b.run)
	return b
}

func (b *Balancer) dialersToCheck() []Dialer {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ds := make([]Dialer, len(b.dialers))
	copy(ds, b.dialers)
	return ds
}

// Reset closes existing dialers and replaces them with new ones.
func (b *Balancer) Reset(dialers ...Dialer) {
	log.Debugf("Resetting with %d dialers", len(dialers))
	var dls SortedDialers
	var tdls SortedDialers

	b.mu.Lock()
	oldDialers := b.dialers
	for _, dl := range dialers {
		dls = append(dls, dl)

		if dl.Trusted() {
			tdls = append(tdls, dl)
		}
	}
	b.dialers = dls
	b.trusted = tdls
	b.mu.Unlock()
	for _, dl := range oldDialers {
		dl.Stop()
	}
}

// Dial dials (network, addr) using one of the currently active configured
// Dialers. The dialer is chosen based on the following ordering:
//
// - succeeding dialers are preferred to failing
// - dialers whose bandwidth is unknown are preferred to those whose bandwidth
//   is known (in order to collect data)
// - faster dialers (based on bandwidth / latency) are preferred to slower ones
//
// Only Trusted Dialers are used to dial HTTP hosts.
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

	dialers, pickErr := b.pickDialers(trustedOnly)
	if pickErr != nil {
		return nil, pickErr
	}

	attempts := dialAttempts
	if attempts > len(dialers) {
		attempts = len(dialers)
	}
	for i := 0; i < attempts; i++ {
		d := dialers[i]
		log.Tracef("Dialing %s://%s with %s", network, addr, d.Label())

		conn, err := b.dialWithTimeout(d, network, addr)
		if err != nil {
			log.Error(errors.New("Unable to dial via %v to %s://%s: %v on pass %v...continuing", d.Label(), network, addr, err, i))
			continue
		}
		// Please leave this at Debug level, as it helps us understand performance
		// issues caused by a poor proxy being selected.
		log.Debugf("Successfully dialed via %v to %v://%v on pass %v", d.Label(), network, addr, i)
		select {
		case b.onActiveDialer <- d:
		default:
		}
		return conn, nil
	}
	return nil, fmt.Errorf("Still unable to dial %s://%s after %d attempts", network, addr, dialAttempts)
}

// OnActiveDialer returns the channel of the last dialer the balancer was using.
// Can be called only once.
func (b *Balancer) OnActiveDialer() <-chan Dialer {
	return b.onActiveDialer
}

func (b *Balancer) dialWithTimeout(d Dialer, network, addr string) (net.Conn, error) {
	limit := b.nextTimeout.GetDuration()
	timer := time.NewTimer(limit)
	var conn net.Conn
	var err error
	// to synchronize access of conn and err between outer and inner goroutine
	chDone := make(chan bool)
	t := time.Now()
	ops.Go(func() {
		conn, err = d.Dial(network, addr)
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
				log.Debugf("Reset balancer dial timeout because dialer %s suddenly slows down", d.Label())
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
	time.Sleep(5 * time.Second)
	b.doPrintStats()
	t := time.NewTicker(15 * time.Second)
	for {
		select {
		case <-b.stopStatsCh:
			t.Stop()
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
	dialersCopy := make(SortedDialers, len(b.dialers))
	copy(dialersCopy, b.dialers)
	sort.Sort(dialersCopy)
	log.Debug("-------------------------- Dialer Stats -----------------------")
	for _, d := range dialersCopy {
		log.Debugf("%s  S: %4d / %4d (%d)\tF: %4d / %4d (%d)\tL: %5.0fms\tBW: %3.2fMbps", d.JustifiedLabel(), d.Successes(), d.Attempts(), d.ConsecSuccesses(), d.Failures(), d.Attempts(), d.ConsecFailures(), d.EstLatency().Seconds()*1000, d.EstBandwidth())
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

func (b *Balancer) pickDialers(trustedOnly bool) ([]Dialer, error) {
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
	result := make([]Dialer, len(dialers))
	copy(result, dialers)
	return result, nil
}

type SortedDialers []Dialer

func (d SortedDialers) Len() int { return len(d) }

func (d SortedDialers) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}

func (d SortedDialers) Less(i, j int) bool {
	a, b := d[i], d[j]

	// Prefer the succeeding proxy
	aSucceeding, bSucceeding := a.Succeeding(), b.Succeeding()
	if aSucceeding && !bSucceeding {
		return true
	}
	if !aSucceeding && bSucceeding {
		return false
	}
	if !aSucceeding && !bSucceeding {
		// If both are failing, sort randomly so that we have the best chance of
		// finding a dialer that works.
		return rand.Float64() < 0.5
	}

	// while proxy's bandwidth is unknown, it should get traffic so that we can
	// ascertain the bandwidth
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
		return strings.Compare(a.Label(), b.Label()) < 0
	}

	// divide bandwidth by latency to determine how to sort
	ela, elb := a.EstLatency().Seconds(), b.EstLatency().Seconds()
	return float64(eba)/ela > float64(ebb)/elb
}
