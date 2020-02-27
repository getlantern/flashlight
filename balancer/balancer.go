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

	humanize "github.com/dustin/go-humanize"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/golog"
)

const (
	// NetworkConnect is a pseudo network name to instruct the dialer to establish
	// a CONNECT tunnel to the proxy.
	NetworkConnect = "connect"
	// NetworkPersistent is a pseudo network name to instruct the dialer to
	// signal the proxy to establish a persistent HTTP connection over which
	// one or more HTTP requests can be sent directly.
	NetworkPersistent = "persistent"

	connectivityRechecks = 3

	printStatsInterval = 10 * time.Second
)

var (
	log = golog.LoggerFor("balancer")

	recheckInterval = 2 * time.Second
	evalInterval    = time.Minute
)

// Dialer provides the ability to dial a proxy and obtain information needed to
// effectively load balance between dialers.
type Dialer interface {
	// DialContext dials out to the given origin. failedUpstream indicates whether
	// this was an upstream error (as opposed to errors connecting to the proxy).
	DialContext(ctx context.Context, network, addr string) (conn net.Conn, failedUpstream bool, err error)

	// Name returns the name for this Dialer
	Name() string

	// Label returns a label for this Dialer (includes Name plus more).
	Label() string

	// JustifiedLabel is like Label() but with elements justified for line-by
	// -line display.
	JustifiedLabel() string

	// Location returns the country code, country name and city name of the
	// dialer, in this order.
	Location() (string, string, string)

	// Protocol returns a string representation of the protocol used by this
	// Dialer.
	Protocol() string

	// Addr returns the address for this Dialer
	Addr() string

	// Trusted indicates whether or not this dialer is trusted
	Trusted() bool

	// NumPreconnecting returns the number of pending preconnect requests.
	NumPreconnecting() int

	// NumPreconnected returns the number of preconnected connections.
	NumPreconnected() int

	// MarkFailure marks a dial failure on this dialer.
	MarkFailure()

	// EstRTT provides a round trip delay time estimate, similar to how RTT is
	// estimated in TCP (https://tools.ietf.org/html/rfc6298)
	EstRTT() time.Duration

	// EstBandwidth provides the estimated bandwidth in Mbps
	EstBandwidth() float64

	// EstSuccessRate returns the estimated success rate dialing this dialer.
	EstSuccessRate() float64

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

	// Probe performs active probing of the proxy to better understand
	// connectivity and performance. If forPerformance is true, the dialer will
	// probe more and with bigger data in order for bandwidth estimation to
	// collect enough data to make a decent estimate. Probe returns true if it was
	// successfully able to communicate with the Proxy.
	Probe(forPerformance bool) bool
	// ProbeStats returns probe related stats for the dialer which can be used
	// to estimate the overhead of active probling.
	ProbeStats() (successes uint64, successKBs uint64, failures uint64, failedKBs uint64)

	// DataSent returns total bytes of application data sent to connections
	// created via this dialer.
	DataSent() uint64
	// DataRecv returns total bytes of application data received from
	// connections created via this dialer.
	DataRecv() uint64

	// Stop stops background processing for this Dialer.
	Stop()

	// Ping performs an ICMP ping of the proxy used by this dialer
	Ping()
}

type dialStats struct {
	success int64
	failure int64
}

// Balancer balances connections among multiple Dialers.
type Balancer struct {
	mu                  sync.RWMutex
	overallDialTimeout  time.Duration
	dialers             sortedDialers
	trusted             sortedDialers
	sessionStats        map[string]*dialStats
	lastReset           time.Time
	chEvalDialers       chan struct{}
	closeOnce           sync.Once
	closeCh             chan struct{}
	onActiveDialer      chan Dialer
	priorTopDialer      Dialer
	hasSucceedingDialer chan bool
	HasSucceedingDialer <-chan bool
	configured          chan struct{}
	closeConfiguredOnce sync.Once
}

// New creates a new Balancer using the supplied Dialers.
func New(overallDialTimeout time.Duration, dialers ...Dialer) *Balancer {
	// a small alpha to gradually adjust timeout based on performance of all
	// dialers
	hasSucceedingDialer := make(chan bool, 1000)
	b := &Balancer{
		overallDialTimeout:  overallDialTimeout,
		closeCh:             make(chan struct{}),
		chEvalDialers:       make(chan struct{}, 1),
		onActiveDialer:      make(chan Dialer, 1),
		hasSucceedingDialer: hasSucceedingDialer,
		HasSucceedingDialer: hasSucceedingDialer,
		configured:          make(chan struct{}),
	}

	b.initOpsContext()

	// TODO: remove or optimize the periodical probing
	ops.Go(b.periodicallyProbeDialers)
	ops.Go(b.periodicallyPrintStats)
	ops.Go(b.evalDialersLoop)
	ops.Go(b.KeepLookingForSucceedingDialer)
	if len(dialers) > 0 {
		b.Reset(dialers)
	}
	return b
}

// Reset closes existing dialers and replaces them with new ones.
func (b *Balancer) Reset(dialers []Dialer) {
	if len(dialers) > 0 {
		defer b.closeConfiguredOnce.Do(func() { close(b.configured) })
	}

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
	recordTopDialer(b.sortDialers())

	for _, dl := range oldDialers {
		dl.Stop()
	}

	b.printStats()
	b.requestEvalDialers("Resetting balancer")
}

// ResetFromExisting Resets using the existing dialers (useful when you want to
// force redialing).
func (b *Balancer) ResetFromExisting() {
	log.Debugf("Resetting from existing dialers")
	b.mu.Lock()
	dialers := b.dialers
	b.mu.Unlock()
	b.Reset(dialers)
}

// Dial dials (network, addr) using one of the currently active configured
// Dialers. The dialer is chosen based on the following ordering:
//
// - succeeding dialers are preferred to failing
// - dialers whose bandwidth is unknown are preferred to those whose bandwidth
//   is known (in order to collect data)
// - faster dialers (based on bandwidth / RTT) are preferred to slower ones
//
// Only Trusted Dialers are used to dial HTTP hosts.
//
// Dial looks through the proxy connections based on the above ordering and
// dial with the first available. If none are available, it keeps cycling
// through the list in priority order until it finds one. It will keep trying
// for up to 30 seconds, at which point it gives up.
//
// Blocks until dialers are available on the balancer (configured via the New
// constructor or b.Reset).
func (b *Balancer) Dial(network, addr string) (net.Conn, error) {
	return b.DialContext(context.Background(), network, addr)
}

// DialContext is same as Dial but uses the provided context.
func (b *Balancer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	op := ops.BeginWithBeam("balancer_dial", ctx)
	defer op.End()

	op = ops.Begin("balancer_dial_details")
	defer op.End()

	select {
	case <-b.configured:
	case <-ctx.Done():
		return nil, errors.New("no configured dialers: %v", ctx.Err())
	}

	start := time.Now()
	bd, err := b.newBalancedDial(network, addr)
	if err != nil {
		return nil, op.FailIf(log.Error(err))
	}
	conn, err := bd.dial(ctx, start)
	if err != nil {
		return nil, op.FailIf(log.Error(err))
	}

	op.BalancerDialTime(time.Since(start), nil)
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
	deadline, _ := newCTX.Deadline()
	attempts := 0
	for {
		conn := bd.dialWithDialer(newCTX, bd.dialers[bd.idx], start, attempts)
		if conn != nil {
			return conn, nil
		}
		attempts++
		if time.Now().After(deadline) {
			break
		}
		if !bd.advanceToNextDialer() {
			break
		}
	}

	return nil, fmt.Errorf("Still unable to dial %s://%s after %d attempts", bd.network, bd.addr, attempts)
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

func (bd *balancedDial) dialWithDialer(ctx context.Context, dialer Dialer, start time.Time, attempts int) net.Conn {
	deadline, _ := ctx.Deadline()
	log.Debugf("Dialing %s://%s with %s on pass %v with timeout %v", bd.network, bd.addr, dialer.Label(), attempts, deadline.Sub(time.Now()))
	oldRTT, oldBW := dialer.EstRTT(), dialer.EstBandwidth()
	conn, failedUpstream, err := dialer.DialContext(ctx, bd.network, bd.addr)
	if err != nil {
		bd.onFailure(dialer, failedUpstream, err, attempts)
		return nil
	}
	// Multiplexed dialers don't wait for anything from the proxy when dialing
	// "persistent" connections, so we can't blindly trust such connections as
	// signals of success dialers.
	if bd.network == NetworkPersistent {
		return conn
	}
	// Please leave this at Debug level, as it helps us understand
	// performance issues caused by a poor proxy being selected.
	log.Debugf("Successfully dialed via %v to %v://%v on pass %v (%v)", dialer.Label(), bd.network, bd.addr, attempts, time.Since(start))
	bd.onSuccess(dialer)
	// Reevaluate all dialers if the top dialer performance dramatically changed
	if attempts == 0 {
		switch {
		case dialer.EstRTT() > oldRTT*5:
			reason := fmt.Sprintf("Dialer %s RTT increased from %v to %v",
				dialer.Label(), oldRTT, dialer.EstRTT())
			bd.requestEvalDialers(reason)
		case dialer.EstBandwidth()*5 < oldBW:
			reason := fmt.Sprintf("Dialer %s bandwidth decreased from %v to %v",
				dialer.Label(), oldBW, dialer.EstBandwidth())
			bd.requestEvalDialers(reason)
		default:
		}
	}
	return conn
}

func (bd *balancedDial) onSuccess(dialer Dialer) {
	atomic.AddInt64(&bd.sessionStats[dialer.Label()].success, 1)
	select {
	case bd.onActiveDialer <- dialer:
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

func (bd *balancedDial) onFailure(dialer Dialer, failedUpstream bool, err error, attempts int) {
	continueString := "...continuing"
	if failedUpstream {
		continueString = "...aborting"
	}
	msg := "%v dialing via %v to %s://%s: %v on pass %v%v"
	if failedUpstream {
		log.Debugf(msg,
			"Upstream error", dialer.Label(), bd.network, bd.addr, err, attempts, continueString)
	} else {
		log.Errorf(msg,
			"Unexpected error", dialer.Label(), bd.network, bd.addr, err, attempts, continueString)
	}
	if failedUpstream {
		bd.failedUpstream[bd.idx] = dialer
	} else {
		atomic.AddInt64(&bd.sessionStats[dialer.Label()].failure, 1)
		if attempts == 0 {
			// Whenever top dialer fails, re-evaluate dialers immediately
			// without checking connectivity for faster convergence. A full
			// check and re-evaluating should follow up if the dialer changes,
			// to avoid permanently switching to a slow dialer due to outdated
			// measurements.
			if bd.evalDialers() {
				bd.requestEvalDialers("Top dialer changed because of failing")
			}
		}
	}
}

// OnActiveDialer returns the channel of the last dialer the balancer was using.
// Can be called only once.
func (b *Balancer) OnActiveDialer() <-chan Dialer {
	return b.onActiveDialer
}

// evalDialersLoop keeps running until the balancer is closed. It checks a
// channel every second to see if there are requests to evalulate all dialers,
// runs it, then wait for one minute (randomized) to recheck the channel.
func (b *Balancer) evalDialersLoop() {
	nextEvalTimer := time.NewTimer(0)
	defer nextEvalTimer.Stop()
	chDone := make(chan struct{})
	for {
		select {
		case <-nextEvalTimer.C:
			select {
			case <-b.chEvalDialers:
				ops.Go(func() {
					b.checkConnectivityForAll()
					_ = b.evalDialers()
					chDone <- struct{}{}
				})
			default:
				nextEvalTimer.Reset(evalInterval / 60)
			}
		case <-chDone:
			nextEvalTimer.Reset(randomize(evalInterval))
		case <-b.closeCh:
			return
		}
	}
}

// evalDialers re-orders dailers and reports/logs the result. It returns true
// if the top dialer changed.
func (b *Balancer) evalDialers() (changed bool) {
	dialers := b.sortDialers()
	if len(dialers) < 2 {
		// nothing to do
		return
	}
	newTopDialer := dialers[0]
	op := ops.Begin("proxy_selection_stability")
	defer op.End()
	b.mu.RLock()
	priorTopDialer := b.priorTopDialer
	b.mu.RUnlock()
	if priorTopDialer == nil {
		op.SetMetricSum("top_dialer_initialized", 1)
		log.Debugf("Top dialer initialized to %v", newTopDialer.Label())
	} else if newTopDialer.Name() == priorTopDialer.Name() {
		op.SetMetricSum("top_dialer_unchanged", 1)
		log.Debug("Top dialer unchanged")
		return
	} else {
		changed = true
		op.SetMetricSum("top_dialer_changed", 1)
		reason := "performance"
		if !priorTopDialer.Succeeding() {
			reason = "failing"
		}
		op.Set("reason", reason)
		log.Debugf("Top dialer changed from %v to %v", priorTopDialer.Label(), newTopDialer.Label())
		recordTopDialer(dialers)
	}
	b.mu.Lock()
	b.priorTopDialer = newTopDialer
	b.mu.Unlock()

	// Print stats immediately after dialer initialized / changed so we have an
	// idea what caused it.
	b.printStats()
	return
}

func (b *Balancer) checkConnectivityForAll() {
	if common.InStealthMode() {
		log.Debug("In stealth mode, not checking connectivity")
		return
	}
	dialers := b.copyOfDialers()
	if len(dialers) < 2 {
		// nothing to do
		return
	}
	log.Debugf("Rechecking connectivity for %d dialers", len(dialers))
	var wg sync.WaitGroup
	wg.Add(len(dialers))
	for _, _d := range dialers {
		d := _d
		ops.Go(func() {
			for i := 0; i < connectivityRechecks; i++ {
				d.Probe(false)
				time.Sleep(randomize(recheckInterval))
			}
			wg.Done()
		})
	}
	wg.Wait()
}

func (b *Balancer) requestEvalDialers(reason string) {
	select {
	case b.chEvalDialers <- struct{}{}:
		log.Debug(reason + ", re-evaluating all dialers")
	default:
	}
}

// Close closes this Balancer, stopping all background processing. You must call
// Close to avoid leaking goroutines.
func (b *Balancer) Close() {
	b.closeOnce.Do(func() {
		b.Reset([]Dialer{})
		close(b.closeCh)
	})
}

func (b *Balancer) periodicallyProbeDialers() {
	t := time.NewTimer(0)
	defer t.Stop()
	for {
		t.Reset(randomize(10 * time.Minute))
		select {
		case <-t.C:
			log.Debugf("Start periodical probing")
			b.checkConnectivityForAll()
			log.Debugf("End periodical probing")
		case <-b.closeCh:
			return
		}
	}
}

func (b *Balancer) periodicallyPrintStats() {
	ticker := time.NewTicker(printStatsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.printStats()
		case <-b.closeCh:
			return
		}
	}
}

func (b *Balancer) printStats() {
	b.mu.Lock()
	dialers := b.dialers
	sessionStats := b.sessionStats
	lastReset := b.lastReset
	b.mu.Unlock()
	log.Debugf("----------- Dialer Stats (%v) -----------", time.Since(lastReset))
	rank := float64(1)
	for _, d := range dialers {
		estRTT := d.EstRTT().Seconds()
		estBandwidth := d.EstBandwidth()
		ds := sessionStats[d.Label()]
		sessionAttempts := atomic.LoadInt64(&ds.success) + atomic.LoadInt64(&ds.failure)
		probeSuccesses, probeSuccessKBs, probeFailures, probeFailedKBs := d.ProbeStats()
		log.Debugf("%s  P:%3d  R:%3d  A: %4d(%5d)  S: %4d(%5d)  CS: %3d  F: %4d(%5d)  CF: %3d  R: %4.3f  L: %4.0fms  B: %6.2fMbps  T: %7s/%7s  P: %3d(%3dkb)/%3d(%3dkb)",
			d.JustifiedLabel(),
			d.NumPreconnected(),
			d.NumPreconnecting(),
			sessionAttempts, d.Attempts(),
			atomic.LoadInt64(&ds.success), d.Successes(), d.ConsecSuccesses(),
			atomic.LoadInt64(&ds.failure), d.Failures(), d.ConsecFailures(),
			d.EstSuccessRate(),
			estRTT*1000, estBandwidth,
			humanize.Bytes(d.DataSent()), humanize.Bytes(d.DataRecv()),
			probeSuccesses, probeSuccessKBs, probeFailures, probeFailedKBs)
		host, _, _ := net.SplitHostPort(d.Addr())
		// Report stats to borda
		op := ops.Begin("proxy_rank").
			ProxyName(d.Name()).
			Set("proxy_host", host).
			SetMetricAvg("rank", rank).
			SetMetricAvg("est_rtt_ms", estRTT*1000)
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

	if dialers.Len() == 0 {
		if trustedOnly {
			return nil, nil, fmt.Errorf("No trusted dialers")
		}
		return nil, nil, fmt.Errorf("No dialers")
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
	dialers := SortDialers(b.copyOfDialers())
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

	return dialers
}

func (b *Balancer) KeepLookingForSucceedingDialer() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	defer close(b.hasSucceedingDialer)

	for {
		select {
		case <-ticker.C:
			b.mu.RLock()
			dialers := b.dialers
			b.mu.RUnlock()
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
		case <-b.closeCh:
			return
		}
	}
}

// PingProxies pings the client's proxies.
func (bal *Balancer) PingProxies() {
	dialers := bal.copyOfDialers()
	for _, dialer := range dialers {
		go dialer.Ping()
	}
}

func recordTopDialer(sortedDialers []Dialer) {
	ops.SetGlobal("num_proxies", len(sortedDialers))

	if len(sortedDialers) == 0 {
		ops.SetGlobal("top_proxy_name", nil)
		ops.SetGlobal("top_dc", nil)
		ops.SetGlobal("top_proxy_protocol", nil)
		return
	}

	dialer := sortedDialers[0]
	name, dc := ops.ProxyNameAndDC(dialer.Name())
	if name != "" {
		ops.SetGlobal("top_proxy_name", name)
	}
	if dc != "" {
		ops.SetGlobal("top_dc", dc)
	}
	ops.SetGlobal("top_proxy_protocol", dialer.Protocol())
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

// This is how the dialers are re-ordered. It's based on the assumption that
// the succeeding status and RTT are up-to-date when sorting (but sensitive to
// packet loss on the wire), while the less-updated estimated bandwidth can be
// a hint when RTT is somewhat comparable.
func (d sortedDialers) Less(i, j int) bool {
	a, b := d[i], d[j]

	// Prefer the proxy with higher success rate
	rateA, rateB := a.EstSuccessRate(), b.EstSuccessRate()
	if rateA-rateB > 0.1 {
		return true
	} else if rateB-rateA > 0.1 {
		return false
	}
	if rateA < 0.1 && rateB < 0.1 {
		// If both have very low success rate, sort randomly so that we have the best chance of
		// finding a dialer that works.
		return rand.Float64() < 0.5
	}

	eba, ebb := a.EstBandwidth(), b.EstBandwidth()
	// should avoid sending traffic to proxy if bandwidth is unknown. The
	// dialer will take care of probing for bandwidth when starts up.
	ebaKnown, ebbKnown := eba != 0, ebb != 0
	if ebaKnown && !ebbKnown {
		return true
	}
	if !ebaKnown && ebbKnown {
		return false
	}

	ela, elb := a.EstRTT().Seconds(), b.EstRTT().Seconds()
	// when RTT differs significantly, choose the one with smaller RTT.
	if ela*3 < elb {
		return true
	}
	if elb*3 < ela {
		return false
	}

	// bandwidth is known for neither proxy and RTT is comparable, sort by
	// label to keep sending traffic to same proxy until we know bandwidth.
	if !ebaKnown && !ebbKnown {
		return strings.Compare(a.Label(), b.Label()) < 0
	}

	// divide bandwidth by rtt to determine how to sort
	return float64(eba)/ela > float64(ebb)/elb
}

func randomize(d time.Duration) time.Duration {
	return d/2 + time.Duration(rand.Int63n(int64(d)))
}

func (b *Balancer) initOpsContext() {
	ops.SetGlobalDynamic("balancer_metrics", func() interface{} {
		metrics := make(map[string]interface{}, 9)
		setProxy := func(prefix string, dialer Dialer) {
			name, dc := ops.ProxyNameAndDC(dialer.Name())
			metrics[prefix+"_proxy"] = name
			metrics[prefix+"_dc"] = dc
			metrics[prefix+"_protocol"] = dialer.Protocol()
		}

		b.mu.RLock()
		dialers := b.dialers
		sessionStats := b.sessionStats
		b.mu.RUnlock()

		if len(dialers) == 0 {
			// no dialers yet, ignore
			return metrics
		}

		var topSession Dialer
		topSessionAttempts := int64(-1)
		var topAllTime Dialer
		topAllTimeAttempts := int64(-1)
		for _, dialer := range dialers {
			ds := sessionStats[dialer.Label()]
			sessionSuccesses := atomic.LoadInt64(&ds.success)
			sessionFailures := atomic.LoadInt64(&ds.failure)
			sessionAttempts := sessionSuccesses + sessionFailures
			if sessionAttempts > topSessionAttempts {
				topSession = dialer
				topSessionAttempts = sessionAttempts
			}
			allTimeAttempts := dialer.Attempts()
			if allTimeAttempts > topAllTimeAttempts {
				topAllTime = dialer
				topAllTimeAttempts = allTimeAttempts
			}
		}

		setProxy("top_current", dialers[0])
		setProxy("top_session", topSession)
		setProxy("top_all_time", topAllTime)

		return metrics
	})
}
