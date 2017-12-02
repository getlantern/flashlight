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

	// DialContext dials out to the given origin. recoverable indicates whether
	// it's possible to recover from this error by dialing again (either on the
	// same or a different dialer).
	DialContext(ctx context.Context, network, addr string) (conn net.Conn, recoverable bool, err error)

	// ExpiresAt indicates when this proxy connection is no longer usable
	ExpiresAt() time.Time
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

	// Preconnected() returns a channel from which we can obtain
	// ProxyConnections.
	Preconnected() <-chan ProxyConnection

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

	// CheckConnectivity checks connectivity to proxy and updates latency and
	// attempts, successes and failures accordingly. It returns true if the check
	// was successful. It should use a timeout internally to avoid blocking
	// indefinitely.
	CheckConnectivity() bool

	// Probe performs active probing of the proxy to better understand
	// connectivity and performance. If forPerformance is true, the dialer will
	// probe more and with bigger data in order for bandwidth estimation to
	// collect enough data to make a decent estimate.
	Probe(forPerformance bool)

	// Stop stops background processing for this Dialer.
	Stop()
}

type dialStats struct {
	success int64
	failure int64
	expired int64
}

// Balancer balances connections among multiple Dialers.
type Balancer struct {
	mu                              sync.RWMutex
	preconnectedDialTimeout         time.Duration
	overallDialTimeout              time.Duration
	dialers                         sortedDialers
	trusted                         sortedDialers
	sessionStats                    map[string]*dialStats
	lastReset                       time.Time
	closeCh                         chan bool
	onActiveDialer                  chan Dialer
	priorTopDialer                  Dialer
	bandwidthKnownForPriorTopDialer bool
	hasSucceedingDialer             chan bool
	HasSucceedingDialer             <-chan bool
}

// New creates a new Balancer using the supplied Dialers.
func New(preconnectedDialTimeout, overallDialTimeout time.Duration, dialers ...Dialer) *Balancer {
	// a small alpha to gradually adjust timeout based on performance of all
	// dialers
	hasSucceedingDialer := make(chan bool, 1000)
	b := &Balancer{
		preconnectedDialTimeout: preconnectedDialTimeout,
		overallDialTimeout:      overallDialTimeout,
		closeCh:                 make(chan bool),
		onActiveDialer:          make(chan Dialer, 1),
		hasSucceedingDialer:     hasSucceedingDialer,
		HasSucceedingDialer:     hasSucceedingDialer,
	}

	b.Reset(dialers...)
	ops.Go(b.run)
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

	b.mu.Lock()
	oldDialers := b.dialers
	b.dialers = dls
	b.sessionStats = sessionStats
	b.lastReset = time.Now()
	b.mu.Unlock()
	b.sortDialers()

	for _, dl := range oldDialers {
		dl.Stop()
	}
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
	op := ops.Begin("balancer_dial")
	defer op.End()

	start := time.Now()
	bd, err := b.newBalancedDial(ctx, network, addr, start)
	if err != nil {
		return nil, op.FailIf(log.Error(err))
	}
	conn, err := bd.dial()
	if err != nil {
		return nil, op.FailIf(log.Error(err))
	}
	op.BalancerDialTime(time.Now().Sub(start), nil)
	return conn, err
}

type balancedDial struct {
	*Balancer
	ctx           context.Context
	network       string
	addr          string
	start         time.Time
	sessionStats  map[string]*dialStats
	dialers       []Dialer
	preconnected  []<-chan ProxyConnection
	unrecoverable map[int]Dialer
	attempts      int
	idx           int
}

func (b *Balancer) newBalancedDial(ctx context.Context, network string, addr string, start time.Time) (*balancedDial, error) {
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

	preconnected := make([]<-chan ProxyConnection, 0, len(dialers))
	for _, dialer := range dialers {
		preconnected = append(preconnected, dialer.Preconnected())
	}

	unrecoverable := make(map[int]Dialer, len(preconnected))

	return &balancedDial{
		Balancer:      b,
		ctx:           ctx,
		network:       network,
		addr:          addr,
		start:         start,
		sessionStats:  sessionStats,
		dialers:       dialers,
		preconnected:  preconnected,
		unrecoverable: unrecoverable,
	}, nil
}

func (bd *balancedDial) dial() (conn net.Conn, err error) {
	for {
		pc := bd.nextPreconnected()
		if pc == nil {
			// no more proxy connections available, stop
			break
		}

		bd.attempts++
		var recoverable bool
		conn, recoverable, err = bd.dialWithTimeout(pc)

		if err != nil {
			bd.onFailure(pc, recoverable, err)
			if !bd.advance() {
				break
			}
			continue
		}

		bd.onSuccess(pc)
		return conn, nil
	}

	return nil, fmt.Errorf("Still unable to dial %s://%s after %d attempts", bd.network, bd.addr, bd.attempts)
}

func (bd *balancedDial) nextPreconnected() ProxyConnection {
	for {
		if time.Now().Sub(bd.start) > bd.overallDialTimeout {
			// reached overall timeout, stop
			return nil
		}

		pcs := bd.preconnected[bd.idx]
		select {
		case pc := <-pcs:
			// got a proxy connection
			if pc.ExpiresAt().Before(time.Now()) {
				// expired dialer, discard and try next from same proxy
				atomic.AddInt64(&bd.sessionStats[pc.Label()].expired, 1)
				continue
			}
			return pc
		default:
			// no proxy connections, tell dialer to preconnect so we'll
			// hopefully get something on the next pass, and keep looking
			bd.dialers[bd.idx].Preconnect()
			if !bd.advance() {
				return nil
			}
		}
	}
}

func (bd *balancedDial) advance() bool {
	if len(bd.unrecoverable) == len(bd.preconnected) {
		// no more recoverable dialers
		return false
	}
	for {
		bd.idx++
		if bd.idx >= len(bd.preconnected) {
			bd.idx = 0
			time.Sleep(250 * time.Millisecond)
		}
		if bd.unrecoverable[bd.idx] != nil {
			// this dialer is not recoverable, don't use it
			continue
		}
		return true
	}
}

func (bd *balancedDial) onSuccess(pc ProxyConnection) {
	atomic.AddInt64(&bd.sessionStats[pc.Label()].success, 1)

	// Preconnect a couple of times to keep preconnected queue full
	pc.Preconnect()
	select {
	case bd.onActiveDialer <- pc:
	default:
	}

	// Mark dialers with upstream errors with failure, since we found a
	// dialer that doesn't suffer from an upstream error. An example of
	// when this might happen is if some dialers have upstream network
	// connectivity issues that prevent them from resolving or connecting
	// to the origin, but other dialers don't suffer from the same issues.
	for _, d := range bd.unrecoverable {
		atomic.AddInt64(&bd.sessionStats[d.Label()].failure, 1)
		d.MarkFailure()
	}
}

func (bd *balancedDial) onFailure(pc ProxyConnection, recoverable bool, err error) {
	recoverableString := "...aborting"
	if recoverable {
		recoverableString = "...continuing"
	}
	log.Errorf("Unable to dial via %v to %s://%s: %v on pass %v%v",
		pc.Label(), bd.network, bd.addr, err, bd.attempts, recoverableString)
	if recoverable {
		atomic.AddInt64(&bd.sessionStats[pc.Label()].failure, 1)
	} else {
		bd.unrecoverable[bd.idx] = pc
	}
}

// OnActiveDialer returns the channel of the last dialer the balancer was using.
// Can be called only once.
func (b *Balancer) OnActiveDialer() <-chan Dialer {
	return b.onActiveDialer
}

func (bd *balancedDial) dialWithTimeout(pc ProxyConnection) (net.Conn, bool, error) {
	log.Debugf("Dialing %s://%s with %s on pass %v", bd.network, bd.addr, pc.Label(), bd.attempts)
	// caps the context deadline to maxDialTimeout
	newCTX, cancel := context.WithTimeout(bd.ctx, bd.preconnectedDialTimeout)
	defer cancel()
	start := time.Now()
	conn, recoverable, err := pc.DialContext(newCTX, bd.network, bd.addr)
	if err == nil {
		// Please leave this at Debug level, as it helps us understand
		// performance issues caused by a poor proxy being selected.
		log.Debugf("Successfully dialed via %v to %v://%v on pass %v (%v)", pc.Label(), bd.network, bd.addr, bd.attempts, time.Since(start))
	}
	return conn, recoverable, err
}

func (b *Balancer) run() {
	ticker := time.NewTicker(evalDialersInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.evalDialers()

		case <-b.closeCh:
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

func (b *Balancer) evalDialers() {
	dialers := b.copyOfDialers()
	if len(dialers) == 0 {
		return
	}

	// First do a tentative sort
	sort.Sort(dialers)
	newTopDialer := dialers[0]
	bandwidthKnownForNewTopDialer := newTopDialer.EstBandwidth() > 0

	// If we've had a change at the top of the order, let's recheck latencies to
	// see if it's just due to general network conditions degrading.
	checkNeeded := b.bandwidthKnownForPriorTopDialer &&
		bandwidthKnownForNewTopDialer &&
		newTopDialer != b.priorTopDialer
	if checkNeeded {
		log.Debugf("Top dialer changed from %v to %v, checking connectivity for all dialers to get updated latencies", b.priorTopDialer.Name(), newTopDialer.Name())
		checkConnectivityForAll(dialers)
		sort.Sort(dialers)
		log.Debugf("Finished checking connectivity for all dialers, resulting in top dialer: %v", dialers[0].Name())
	}

	// Now that we have updated metrics, sort dialers for real
	b.sortDialers()
	b.priorTopDialer = newTopDialer
	b.bandwidthKnownForPriorTopDialer = bandwidthKnownForNewTopDialer
}

func checkConnectivityForAll(dialers sortedDialers) {
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
}

func checkConnectivityFor(d Dialer) {
	for i := 0; i < connectivityRechecks; i++ {
		d.CheckConnectivity()
		time.Sleep(randomize(recheckInterval))
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

func (b *Balancer) printStats(dialers sortedDialers, sessionStats map[string]*dialStats, lastReset time.Time) {
	log.Debugf("----------- Dialer Stats (%v) -----------", time.Now().Sub(lastReset))
	rank := float64(1)
	for _, d := range dialers {
		estLatency := d.EstLatency().Seconds()
		estBandwidth := d.EstBandwidth()
		ds := sessionStats[d.Label()]
		sessionAttempts := atomic.LoadInt64(&ds.success) + atomic.LoadInt64(&ds.failure) + atomic.LoadInt64(&ds.expired)
		log.Debugf("%s  A: %5d (%6d)\tS: %5d (%6d)\tCS: (%5d)\tF: %5d (%6d)\tCF: %5d\tEXP: %5d\tL: %5.0fms\tBW: %10.2fMbps\t",
			d.JustifiedLabel(),
			sessionAttempts, d.Attempts(),
			atomic.LoadInt64(&ds.success), d.Successes(), d.ConsecSuccesses(),
			atomic.LoadInt64(&ds.failure), d.Failures(), d.ConsecFailures(),
			atomic.LoadInt64(&ds.expired),
			estLatency*1000, estBandwidth)
		host, _, _ := net.SplitHostPort(d.Addr())
		// Report stats to borda
		op := ops.Begin("proxy_rank").
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

func (b *Balancer) sortDialers() {
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
	sessionStats := b.sessionStats
	lastReset := b.lastReset
	b.mu.Unlock()

	b.lookForSucceedingDialer(dialers)
	b.printStats(dialers, sessionStats, lastReset)
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

func randomize(d time.Duration) time.Duration {
	return d/2 + time.Duration(rand.Int63n(int64(d)))
}
