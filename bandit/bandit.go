package bandit

import (
	"context"
	"io"
	"math/rand"
	"net"
	"sync/atomic"
	"time"

	bandit "github.com/alextanhongpin/go-bandit"
	"github.com/getlantern/flashlight/v7/stats"
	"github.com/getlantern/golog"
)

var log = golog.LoggerFor("bandit")

const (
	// NetworkConnect is a pseudo network name to instruct the dialer to establish
	// a CONNECT tunnel to the proxy.
	NetworkConnect = "connect"
	// NetworkPersistent is a pseudo network name to instruct the dialer to
	// signal the proxy to establish a persistent HTTP connection over which
	// one or more HTTP requests can be sent directly.
	NetworkPersistent = "persistent"
)

// BanditDialer is responsible for continually choosing the optimized dialer.
type BanditDialer struct {
	dialers      []Dialer
	bandit       *bandit.EpsilonGreedy
	onError      func(error, bool)
	onSuccess    func(Dialer)
	statsTracker stats.Tracker
}

// Options are the options used to create a new bandit
type Options struct {
	// The available dialers to use when creating a new bandit
	Dialers []Dialer

	// OnError is the onError callback that is called when bandit encounters a dial error
	OnError func(error, bool)

	// OnSuccess is the callback that is called by bandit after a successful dial.
	OnSuccess func(Dialer)

	// StatsTracker is a stats.Tracker bandit should be configured to use (a callback that is called
	// when a dialer is selected)
	StatsTracker stats.Tracker
}

// New creates a new bandit given the available dialers and options with
// callbacks to be called when a dialer is selected, an error occurs, etc.
func New(opts Options) (*BanditDialer, error) {
	if opts.OnError == nil {
		opts.OnError = func(error, bool) {}
	}
	if opts.OnSuccess == nil {
		opts.OnSuccess = func(Dialer) {}
	}
	if opts.StatsTracker == nil {
		opts.StatsTracker = stats.NewNoop()
	}

	dialers := opts.Dialers
	log.Debugf("Creating bandit with %d dialers", len(dialers))
	b, err := bandit.NewEpsilonGreedy(0.1, nil, nil)
	if err != nil {
		log.Errorf("unable to create bandit: %v", err)
		return nil, err
	}

	b.Init(len(dialers))
	parallelDial(dialers, b)
	dialer := &BanditDialer{
		dialers:      dialers,
		bandit:       b,
		onError:      opts.OnError,
		onSuccess:    opts.OnSuccess,
		statsTracker: opts.StatsTracker,
	}

	return dialer, nil
}

// parallelDial dials all the dialers in parallel to measure pure connectivity
// as a means of seeding the bandit with initial data. This should
// help weed out dialers/proxies censored users can't connect to at all.
func parallelDial(dialers []Dialer, bandit *bandit.EpsilonGreedy) {
	for index, d := range dialers {
		// Note that the index of the dialer in our list is the index of the arm
		// in the bandit and that the bandit is concurrent-safe.
		go func(dialer Dialer, index int) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			start := time.Now()
			conn, err := dialer.DialProxy(ctx)
			defer func() {
				if conn != nil {
					conn.Close()
				}
			}()
			if err != nil {
				log.Debugf("Dialer %v failed: %v", dialer.Name(), err)
				bandit.Update(index, 0)
				return
			}
			log.Debugf("Dialer %v succeeded in %v seconds", dialer.Name(), time.Since(start)*time.Second)
			bandit.Update(index, 1)
		}(d, index)
	}
}

func (o *BanditDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	deadline, _ := ctx.Deadline()
	log.Debugf("bandit::DialContext::time remaining: %v", time.Until(deadline))
	// We can not create a multi-armed bandit with no arms.
	if len(o.dialers) == 0 {
		return nil, log.Error("Cannot dial with no dialers")
	}

	start := time.Now()
	dialer, chosenArm := o.chooseDialerForDomain(o.dialers, network, addr)

	// We have to be careful here about virtual, multiplexed connections, as the
	// initial TCP dial will have different performance characteristics than the
	// subsequent virtual connection dials.
	log.Debugf("bandit::dialer %d: %s at %v", chosenArm, dialer.Label(), dialer.Addr())
	conn, failedUpstream, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		hasSucceeding := hasSucceedingDialer(o.dialers)
		o.statsTracker.SetHasSucceedingProxy(hasSucceeding)
		o.onError(err, hasSucceeding)

		if !failedUpstream {
			log.Errorf("Dialer %v failed: %v", dialer.Name(), err)
			o.bandit.Update(chosenArm, 0)
		} else {
			log.Debugf("Dialer %v failed upstream...", dialer.Name())
			// This can happen, for example, if the upstream server is down, or
			// if the DNS resolves to localhost, for example. It is also possible
			// that the proxy is blacklisted by upstream sites for some reason,
			// so we have to choose some reasonable value.
			o.bandit.Update(chosenArm, 0.00005)
		}
		return nil, err
	}
	log.Debugf("Dialer %v dialed in %v seconds", dialer.Name(), time.Since(start).Seconds())
	// We don't give any special reward for a successful dial here and just rely on
	// the normalized raw throughput to determine the reward. This is because the
	// reward system takes into account how many tries there have been for a given
	// "arm", so giving a reward here would be double-counting.

	// Tell the dialer to update the bandit with it's throughput after 5 seconds.
	var dataRecv atomic.Uint64
	dt := newDataTrackingConn(conn, &dataRecv)
	time.AfterFunc(secondsForSample*time.Second, func() {
		speed := normalizeReceiveSpeed(dataRecv.Load())
		//log.Debugf("Dialer %v received %v bytes in %v seconds, normalized speed: %v", dialer.Name(), dt.dataRecv, secondsForSample, speed)
		o.bandit.Update(chosenArm, speed)
	})

	countryCode, country, city := dialer.Location()
	previousStats := o.statsTracker.Latest()
	if previousStats.CountryCode != "" && previousStats.CountryCode != countryCode {
		o.statsTracker.SetActiveProxyLocation(
			city,
			country,
			countryCode,
		)
	}
	o.statsTracker.SetHasSucceedingProxy(true)
	o.onSuccess(dialer)
	return dt, err
}

// hasSucceedingDialer checks whether or not any of the given dialers is able to successfully dial our proxies
func hasSucceedingDialer(dialers []Dialer) bool {
	for _, dialer := range dialers {
		if dialer.Succeeding() {
			return true
		}
	}
	return false
}

func (o *BanditDialer) chooseDialerForDomain(dialers []Dialer, network, addr string) (Dialer, int) {
	chosenArm := o.bandit.SelectArm(rand.Float64())
	dialer := dialers[chosenArm]
	if dialer.SupportsAddr(network, addr) {
		return dialer, chosenArm
	}
	chosenArm = differentArm(chosenArm, len(dialers))
	return dialers[chosenArm], chosenArm
}

// Choose a different arm than the one we already have, if possible.
func differentArm(existingArm, numDialers int) int {
	// This selects a new arm randomly, which is preferable to just choosing
	// the next one in the list because that will always be the next dialer
	// after whatever dialer is currently best.
	for i := 0; i < 20; i++ {
		newArm := rand.Intn(numDialers)
		if newArm != existingArm {
			return newArm
		}
	}

	// If random selection doesn't work, just choose the next one.
	return (existingArm + 1) % numDialers
}

const secondsForSample = 6

// A reasonable upper bound for the top expected bytes to receive per second.
// Anything over this will be normalized to over 1.
const topExpectedBps = 125000

func normalizeReceiveSpeed(dataRecv uint64) float64 {
	// Record the bytes in relation to the top expected speed.
	return (float64(dataRecv) / secondsForSample) / topExpectedBps
}

func (o *BanditDialer) Close() {
	log.Debug("Closing all dialers")
	for _, dialer := range o.dialers {
		dialer.Stop()
	}
}

func newDataTrackingConn(conn net.Conn, dataRecv *atomic.Uint64) *dataTrackingConn {
	return &dataTrackingConn{
		Conn:     conn,
		dataRecv: dataRecv,
	}
}

type dataTrackingConn struct {
	net.Conn
	dataRecv *atomic.Uint64
}

func (c *dataTrackingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.dataRecv.Add(uint64(n))
	return n, err
}

// Dialer provides the ability to dial a proxy and obtain information needed to
// effectively load balance between dialers.
type Dialer interface {

	// DialProxy dials the proxy but does not yet dial the origin.
	DialProxy(ctx context.Context) (net.Conn, error)

	// SupportsAddr indicates whether this Dialer supports the given addr. If it does not, the
	// balancer will not attempt to dial that addr with this Dialer.
	SupportsAddr(network, addr string) bool

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

	// DataSent returns total bytes of application data sent to connections
	// created via this dialer.
	DataSent() uint64
	// DataRecv returns total bytes of application data received from
	// connections created via this dialer.
	DataRecv() uint64

	// Stop stops background processing for this Dialer.
	Stop()

	WriteStats(w io.Writer)
}
