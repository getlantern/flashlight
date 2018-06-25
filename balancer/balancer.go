// Package balancer provides load balancing of network connections per
// different strategies.
package balancer

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/golog"
)

const (
	connectivityRechecks = 3

	evalDialersInterval = 10 * time.Second
)

var (
	log = golog.LoggerFor("balancer")

	recheckInterval = 2 * time.Second
)

// ProxyConnection is a pre-established connection to a Proxy which we can
// use to dial out to an origin
type ProxyConnection interface {
	Dialer

	// DialContext dials out to the given origin. failedUpstream indicates whether
	// this was an upstream error (as opposed to errors connecting to the proxy).
	DialContext(ctx context.Context, network, addr string) (conn net.Conn, failedUpstream bool, err error)
}

// Dialer provides the ability to dial a proxy and obtain information needed to
// effectively load balance between dialers.
type Dialer interface {
	// Name returns the name for this Dialer
	Name() string

	// Label returns a label for this Dialer (includes Name plus more).
	Label() string

	// JustifiedLabel is like Label() but with elements justified for line-by
	// -line display.
	JustifiedLabel() string

	// Addr returns the address for this Dialer
	Addr() string

	// Trusted indicates whether or not this dialer is trusted
	Trusted() bool

	// Preconnect tells the dialer to go ahead and preconnect 1 connection (in
	// the background)
	Preconnect()

	// NumPreconnecting returns the number of pending preconnect requests.
	NumPreconnecting() int

	// NumPreconnected returns the number of preconnected connections.
	NumPreconnected() int

	// Preconnected() returns a preconnected ProxyConnection or nil if none is
	// immediately available.
	Preconnected() ProxyConnection

	// MarkFailure marks a dial failure on this dialer.
	MarkFailure()

	// EstLatency provides a latency estimate
	EstLatency() time.Duration

	// EstBandwidth provides the estimated bandwidth in Mbps
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

	// Forces the dialer to reconnect to its proxy server
	ForceRedial()

	// Probe performs active probing of the proxy to better understand
	// connectivity and performance. If forPerformance is true, the dialer will
	// probe more and with bigger data in order for bandwidth estimation to
	// collect enough data to make a decent estimate. Probe returns true if it was
	// successfully able to communicate with the Proxy.
	Probe(forPerformance bool) bool

	// Stop stops background processing for this Dialer.
	Stop()
}

type dialStats struct {
	success int64
	failure int64
}

// Balancer balances connections among multiple Dialers.
type Balancer struct {
	beam_seq                        uint64
	mu                              sync.RWMutex
	evalMx                          sync.Mutex
	overallDialTimeout              time.Duration
	dialers                         sortedDialers
	trusted                         sortedDialers
	sessionStats                    map[string]*dialStats
	lastReset                       time.Time
	recheckConnectivityCh           chan []Dialer
	closeOnce                       sync.Once
	closeCh                         chan interface{}
	onActiveDialer                  chan Dialer
	priorTopDialer                  Dialer
	bandwidthKnownForPriorTopDialer bool
	hasSucceedingDialer             chan bool
	HasSucceedingDialer             <-chan bool
}

// New creates a new Balancer using the supplied Dialers.
func New(overallDialTimeout time.Duration, dialers ...Dialer) *Balancer {
	// a small alpha to gradually adjust timeout based on performance of all
	// dialers
	hasSucceedingDialer := make(chan bool, 1000)
	b := &Balancer{
		overallDialTimeout:    overallDialTimeout,
		recheckConnectivityCh: make(chan []Dialer),
		closeCh:               make(chan interface{}),
		onActiveDialer:        make(chan Dialer, 1),
		hasSucceedingDialer:   hasSucceedingDialer,
		HasSucceedingDialer:   hasSucceedingDialer,
	}

	b.Reset(dialers...)
	ops.Go(b.run)
	ops.Go(b.periodicallyPrintStats)
	ops.Go(b.recheckConnectivity)
	return b
}

// Reset closes existing dialers and replaces them with new ones.
func (b *Balancer) Reset(dialers ...Dialer) {
	log.Debugf("Resetting with %d dialers", len(dialers))
	dls := make(sortedDialers, len(dialers))
	copy(dls, dialers)

	sessionStats := make(map[string]*dialStats, len(dls))
	for _, d := range dls {
		sessionStats[d.Label()] = &dialStats{}
	}

	lastReset := time.Now()
	b.mu.Lock()
	oldDialers := b.dialers
	b.dialers = dls
	b.sessionStats = sessionStats
	b.lastReset = lastReset
	b.mu.Unlock()
	b.sortDialers()

	for _, dl := range oldDialers {
		dl.Stop()
	}

	b.printStats(dialers, sessionStats, lastReset)
}

// ForceRedial forces dialers with long-running connections to reconnect
func (b *Balancer) ForceRedial() {
	log.Debugf("Received request to force redial")
	b.mu.Lock()
	dialers := b.dialers
	b.mu.Unlock()
	for _, dl := range dialers {
		dl.ForceRedial()
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
// Dial looks through the proxy connections based on the above ordering and
// dial with the first available. If none are available, it keeps cycling
// through the list in priority order until it finds one. It will keep trying
// for up to 30 seconds, at which point it gives up.
func (b *Balancer) Dial(network, addr string) (net.Conn, error) {
	return b.DialContext(context.Background(), network, addr)
}

// DialContext is same as Dial but uses the provided context.
func (b *Balancer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	op := ops.Begin("balancer_dial").Set("beam", atomic.AddUint64(&b.beam_seq, 1))
	defer op.End()

	start := time.Now()
	bd, err := b.newBalancedDial(network, addr)
	if err != nil {
		return nil, op.FailIf(log.Error(err))
	}
	conn, err := bd.dial(ctx, start)
	if err != nil {
		return nil, op.FailIf(log.Error(err))
	}

	op.BalancerDialTime(time.Now().Sub(start), nil)
	return conn, nil
}

// balancedDial encapsulates a single dial using the available Dialers
type balancedDial struct {
	*Balancer
	network        string
	addr           string
	sessionStats   map[string]*dialStats
	dialers        []Dialer
	failedUpstream map[int]Dialer
	idx            int
}

func (b *Balancer) newBalancedDial(network string, addr string) (*balancedDial, error) {
	trustedOnly := false
	_, port, _ := net.SplitHostPort(addr)
	// We try to identify HTTP traffic (as opposed to HTTPS) by port and only
	// send HTTP traffic to dialers marked as trusted.
	if port == "" || port == "80" || port == "8080" {
		trustedOnly = true
	}

	dialers, sessionStats, pickErr := b.pickDialers(trustedOnly)
	if pickErr != nil {
		return nil, pickErr
	}

	return &balancedDial{
		Balancer:       b,
		network:        network,
		addr:           addr,
		sessionStats:   sessionStats,
		dialers:        dialers,
		failedUpstream: make(map[int]Dialer, len(dialers)),
	}, nil
}

func (bd *balancedDial) dial(ctx context.Context, start time.Time) (conn net.Conn, err error) {
	newCTX, cancel := context.WithTimeout(ctx, bd.Balancer.overallDialTimeout)
	defer cancel()
	attempts := 0
	failedUpstream := false
	for {
		pc := bd.nextPreconnected(newCTX)
		if pc == nil {
			// no more proxy connections available, stop
			break
		}

		deadline, _ := newCTX.Deadline()
		log.Debugf("Dialing %s://%s with %s on pass %v with timeout %v", bd.network, bd.addr, pc.Label(), attempts, deadline.Sub(time.Now()))
		conn, failedUpstream, err = pc.DialContext(newCTX, bd.network, bd.addr)
		if err == nil {
			// Please leave this at Debug level, as it helps us understand
			// performance issues caused by a poor proxy being selected.
			log.Debugf("Successfully dialed via %v to %v://%v on pass %v (%v)", pc.Label(), bd.network, bd.addr, attempts, time.Since(start))
			bd.onSuccess(pc)
			return conn, nil
		}

		bd.onFailure(pc, failedUpstream, err, attempts)
		attempts++
		if !bd.advanceToNextDialer() {
			break
		}
	}

	return nil, fmt.Errorf("Still unable to dial %s://%s after %d attempts", bd.network, bd.addr, attempts)
}

func (bd *balancedDial) nextPreconnected(ctx context.Context) ProxyConnection {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		curDialer := bd.dialers[bd.idx]
		pc := curDialer.Preconnected()
		if pc != nil {
			// Aggressively preconnect to build up queue
			curDialer.Preconnect()
			return pc
		}
		// no proxy connections, advance to next dialer
		if !bd.advanceToNextDialer() {
			return nil
		}
	}
}

// advanceToNextDialer advances this balancedDial to the next dialer, cycling
// back to the beginning if necessary. If all dialers have failed upstream, this
// method returns false.
func (bd *balancedDial) advanceToNextDialer() bool {
	if len(bd.failedUpstream) == len(bd.dialers) {
		// all dialers failed upstream, give up
		return false
	}

	for {
		bd.idx++
		if bd.idx >= len(bd.dialers) {
			bd.idx = 0
			time.Sleep(250 * time.Millisecond)
		}
		if bd.failedUpstream[bd.idx] != nil {
			// this dialer failed upstream, don't bother trying again
			continue
		}
		return true
	}
}

func (bd *balancedDial) onSuccess(pc ProxyConnection) {
	atomic.AddInt64(&bd.sessionStats[pc.Label()].success, 1)
	select {
	case bd.onActiveDialer <- pc:
	default:
	}

	// Mark dialers with upstream errors with failure, since we found a
	// dialer that doesn't suffer from an upstream error. An example of
	// when this might happen is if some dialers have upstream network
	// connectivity issues that prevent them from resolving or connecting
	// to the origin, but other dialers don't suffer from the same issues.
	for _, d := range bd.failedUpstream {
		atomic.AddInt64(&bd.sessionStats[d.Label()].failure, 1)
		d.MarkFailure()
	}
}

func (bd *balancedDial) onFailure(pc ProxyConnection, failedUpstream bool, err error, attempts int) {
	continueString := "...continuing"
	if failedUpstream {
		continueString = "...aborting"
	}
	msg := "%v dialing via %v to %s://%s: %v on pass %v%v"
	if failedUpstream {
		log.Debugf(msg,
			"Upstream error", pc.Label(), bd.network, bd.addr, err, attempts, continueString)
	} else {
		log.Errorf(msg,
			"Unexpected error", pc.Label(), bd.network, bd.addr, err, attempts, continueString)
	}
	if failedUpstream {
		bd.failedUpstream[bd.idx] = pc
	} else {
		atomic.AddInt64(&bd.sessionStats[pc.Label()].failure, 1)
	}
}

// OnActiveDialer returns the channel of the last dialer the balancer was using.
// Can be called only once.
func (b *Balancer) OnActiveDialer() <-chan Dialer {
	return b.onActiveDialer
}

func (b *Balancer) run() {
	ticker := time.NewTicker(evalDialersInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.evalDialers(true)

		case <-b.closeCh:
			b.mu.Lock()
			oldDialers := b.dialers
			b.dialers = nil
			b.mu.Unlock()
			for _, d := range oldDialers {
				d.Stop()
			}
			return
		}
	}
}

func (b *Balancer) evalDialers(checkAllowed bool) []Dialer {
	b.evalMx.Lock()
	defer b.evalMx.Unlock()

	dialers := b.sortDialers()
	if len(dialers) < 2 {
		// nothing to do
		return dialers
	}
	newTopDialer := dialers[0]
	bandwidthKnownForNewTopDialer := newTopDialer.EstBandwidth() > 0

	// If we've had a change at the top of the order, let's recheck latencies to
	// see if it's just due to general network conditions degrading.
	checkNeeded := checkAllowed && b.bandwidthKnownForPriorTopDialer &&
		bandwidthKnownForNewTopDialer &&
		newTopDialer != b.priorTopDialer
	if checkNeeded {
		log.Debugf("Top dialer changed from %v to %v, checking connectivity for all dialers to get updated latencies", b.priorTopDialer.Name(), newTopDialer.Name())
		b.checkConnectivityForAll(dialers)
	}

	op := ops.Begin("proxy_selection_stability")
	topDialerChanged := b.priorTopDialer != nil && newTopDialer != b.priorTopDialer
	if b.priorTopDialer == nil {
		op.SetMetricSum("top_dialer_initialized", 1)
		log.Debug("Top dialer initialized")
	} else if topDialerChanged {
		op.SetMetricSum("top_dialer_changed", 1)
		reason := "performance"
		if !b.priorTopDialer.Succeeding() {
			reason = "failing"
		}
		op.Set("reason", reason)
		log.Debug("Top dialer changed")
	} else {
		op.SetMetricSum("top_dialer_unchanged", 1)
		log.Debug("Top dialer unchanged")
	}
	op.End()

	b.priorTopDialer = newTopDialer
	b.bandwidthKnownForPriorTopDialer = bandwidthKnownForNewTopDialer

	return dialers
}

func (b *Balancer) checkConnectivityForAll(dialers sortedDialers) {
	select {
	case b.recheckConnectivityCh <- dialers:
		// okay
	default:
		log.Debug("Already rechecking connectivity for all dialers, ignoring duplicate request for checks")
	}
}

func (b *Balancer) recheckConnectivity() {
	for {
		select {
		case <-b.closeCh:
			// closed
			return
		case dialers := <-b.recheckConnectivityCh:
			log.Debugf("Rechecking connectivity for %d dialers", len(dialers))
			var wg sync.WaitGroup
			wg.Add(len(dialers))
			for _, _d := range dialers {
				d := _d
				ops.Go(func() {
					checkConnectivityFor(d)
					wg.Done()
				})
			}
			wg.Wait()
			sortedDialers := b.evalDialers(false)
			if len(sortedDialers) == 0 {
				log.Debug("Finished checking connectivity for all dialers, but no dialers are left!")
			} else {
				log.Debugf("Finished checking connectivity for all dialers, resulting in top dialer: %v", sortedDialers[0].Name())
			}
		}
	}
}

func checkConnectivityFor(d Dialer) {
	for i := 0; i < connectivityRechecks; i++ {
		d.Probe(false)
		time.Sleep(randomize(recheckInterval))
	}
}

// Close closes this Balancer, stopping all background processing. You must call
// Close to avoid leaking goroutines.
func (b *Balancer) Close() {
	b.closeOnce.Do(func() {
		close(b.closeCh)
	})
}

func (b *Balancer) periodicallyPrintStats() {
	ticker := time.NewTicker(evalDialersInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.mu.Lock()
			dialers := b.dialers
			sessionStats := b.sessionStats
			lastReset := b.lastReset
			b.mu.Unlock()
			b.printStats(dialers, sessionStats, lastReset)

		case <-b.closeCh:
			return
		}
	}
}

func (b *Balancer) printStats(dialers sortedDialers, sessionStats map[string]*dialStats, lastReset time.Time) {
	log.Debugf("----------- Dialer Stats (%v) -----------", time.Now().Sub(lastReset))
	rank := float64(1)
	for _, d := range dialers {
		estLatency := d.EstLatency().Seconds()
		estBandwidth := d.EstBandwidth()
		ds := sessionStats[d.Label()]
		sessionAttempts := atomic.LoadInt64(&ds.success) + atomic.LoadInt64(&ds.failure)
		log.Debugf("%s  P:%2d  R:%2d  A: %5d(%6d)  S: %5d(%6d)  CS: %5d  F: %5d(%6d)  CF: %5d  L: %5.0fms  B: %10.2fMbps",
			d.JustifiedLabel(),
			d.NumPreconnected(),
			d.NumPreconnecting(),
			sessionAttempts, d.Attempts(),
			atomic.LoadInt64(&ds.success), d.Successes(), d.ConsecSuccesses(),
			atomic.LoadInt64(&ds.failure), d.Failures(), d.ConsecFailures(),
			estLatency*1000, estBandwidth)
		host, _, _ := net.SplitHostPort(d.Addr())
		// Report stats to borda
		op := ops.Begin("proxy_rank").
			ProxyName(d.Name()).
			Set("proxy_host", host).
			SetMetricAvg("rank", rank).
			SetMetricAvg("est_rtt", estLatency)
		if estBandwidth > 0 {
			op.SetMetricAvg("est_mbps", estBandwidth)
		}
		op.End()
		rank++
	}
	log.Debug("----------- End Dialer Stats -----------")
}

func (b *Balancer) pickDialers(trustedOnly bool) ([]Dialer, map[string]*dialStats, error) {
	b.mu.RLock()
	dialers := b.dialers
	if trustedOnly {
		dialers = b.trusted
	}
	sessionStats := b.sessionStats
	b.mu.RUnlock()

	if !trustedOnly {
		b.lookForSucceedingDialer(dialers)
	}

	if dialers.Len() == 0 {
		if trustedOnly {
			return nil, nil, fmt.Errorf("No trusted dialers")
		}
		return nil, nil, fmt.Errorf("No dialers")
	}

	topDialer := dialers[0]
	for i := 1; i < len(dialers); i++ {
		dialer := dialers[i]
		if dialer.Succeeding() && dialer.EstLatency().Seconds()/topDialer.EstLatency().Seconds() < 0.75 && rand.Float64() < 0.05 {
			// We generally assume that dialers with lower latency could be faster
			// overall, so send a little traffic to them to find out if that's true.
			// Amongst other things, this allows the fastest dialer to eventually
			// recover after a temporary hiccup.
			log.Debugf("Dialer %v has a dramatically lower latency than top dialer %v, randomly moving it to the top of the line", dialer.Name(), topDialer.Name())
			randomized := make([]Dialer, 0, len(dialers))
			randomized = append(randomized, dialer)
			for j, other := range dialers {
				if j != i {
					randomized = append(randomized, other)
				}
			}
			return randomized, sessionStats, nil
		}
	}

	return dialers, sessionStats, nil
}

func (b *Balancer) copyOfDialers() sortedDialers {
	b.mu.RLock()
	_dialers := b.dialers
	b.mu.RUnlock()
	dialers := make(sortedDialers, len(_dialers))
	copy(dialers, _dialers)
	return dialers
}

func (b *Balancer) sortDialers() []Dialer {
	dialers := b.copyOfDialers()
	sort.Sort(dialers)

	trusted := make(sortedDialers, 0, len(dialers))
	for _, d := range dialers {
		if d.Trusted() {
			trusted = append(trusted, d)
		}
	}

	b.mu.Lock()
	b.dialers = dialers
	b.trusted = trusted
	b.mu.Unlock()

	b.lookForSucceedingDialer(dialers)
	return dialers
}

func (b *Balancer) lookForSucceedingDialer(dialers []Dialer) {
	hasSucceedingDialer := false
	for _, dialer := range dialers {
		if dialer.Succeeding() {
			hasSucceedingDialer = true
			break
		}
	}
	select {
	case b.hasSucceedingDialer <- hasSucceedingDialer:
		// okay
	default:
		// channel full
	}
}

func SortDialers(dialers []Dialer) []Dialer {
	sorted := sortedDialers(dialers)
	sort.Sort(sorted)
	return sorted
}

type sortedDialers []Dialer

func (d sortedDialers) Len() int { return len(d) }

func (d sortedDialers) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}

func (d sortedDialers) Less(i, j int) bool {
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
	ebaKnown, ebbKnown := eba != 0, ebb != 0
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

func randomize(d time.Duration) time.Duration {
	return d/2 + time.Duration(rand.Int63n(int64(d)))
}
