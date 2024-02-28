package bandit

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
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
	statsTracker stats.Tracker
}

type BanditError struct {
	attempts int
}

func (be *BanditError) Error() string {
	return fmt.Sprintf("No dialer succeeded after %v attempts", be.attempts)
}

// New creates a new bandit given the available dialers.
func New(dialers []Dialer) (*BanditDialer, error) {
	return NewWithStats(dialers, stats.NewNoop())
}

// NewWithStats creates a new BanditDialer given the available dialers and a
// callback to be called when a dialer is selected.
func NewWithStats(dialers []Dialer, statsTracker stats.Tracker) (*BanditDialer, error) {
	log.Debugf("Creating bandit with %d dialers", len(dialers))
	b, err := bandit.NewEpsilonGreedy(0.1, nil, nil)
	if err != nil {
		log.Errorf("unable to create bandit: %v", err)
		return nil, err
	}

	b.Init(len(dialers))
	return &BanditDialer{
		dialers:      dialers,
		bandit:       b,
		statsTracker: statsTracker,
	}, nil
}

func (o *BanditDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	deadline, _ := ctx.Deadline()
	log.Debugf("bandit::DialContext::time remaining: %v", time.Until(deadline))
	// We can not create a multi-armed bandit with no arms.
	if len(o.dialers) == 0 {
		return nil, log.Error("Cannot dial with no dialers")
	}

	start := time.Now()
	chosenArm := o.bandit.SelectArm(rand.Float64())

	// We have to be careful here about virtual, multiplexed connections, as the
	// initial TCP dial will have different performance characteristics than the
	// subsequent virtual connection dials.
	for i := 0; i < len(o.dialers); i++ {
		select {
		case <-ctx.Done():
			log.Debug("Overall bandit dial context done")
			return nil, ctx.Err()
		default:
		}
		dialer := o.dialers[chosenArm]
		if !dialer.SupportsAddr(network, addr) {
			// If this dialer doesn't allow this domain, we need to choose a different one,
			// but without giving this dialer a low reward.
			chosenArm = differentArm(chosenArm, len(o.dialers), o.bandit)
			continue
		}
		log.Debugf("bandit::dialer %d: %s at %v", chosenArm, dialer.Label(), dialer.Addr())
		dStart := time.Now()
		// Use a separate context for each dialer to ensure that each dialer
		// has a chance to dial.
		dialCtx := ctx
		if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
			partialDeadline, err := partialDeadline(deadline, len(o.dialers)-i)
			if err != nil {
				// There is no more time left in the overall dial timeout, so we
				// should stop trying to dial.
				log.Debug("Could not allocate partial deadline, stopping dialing.")
				return nil, err
			}
			if partialDeadline.Before(deadline) {
				var cancel context.CancelFunc
				dialCtx, cancel = context.WithDeadline(ctx, partialDeadline)
				defer cancel()
			}
		}
		conn, failedUpstream, err := dialer.DialContext(dialCtx, network, addr)
		if err != nil {
			if !failedUpstream {
				log.Errorf("Dialer %v failed in %v: %v", dialer.Name(), time.Since(dStart).Seconds(), err)
				o.bandit.Update(chosenArm, 0)
			} else {
				log.Debugf("Dialer %v failed upstream in %v...", dialer.Name(), time.Since(dStart).Seconds())
				// This can happen, for example, if the upstream server is down, or
				// if the DNS resolves to localhost, for example. It is also possible
				// that the proxy is blacklisted by upstream sites for some reason,
				// so we have to choose some reasonable value.
				o.bandit.Update(chosenArm, 0.00005)
			}
			// Mark the failure and try another.
			chosenArm = differentArm(chosenArm, len(o.dialers), o.bandit)
			continue
		}
		log.Debugf("Dialer %v dialed in %v seconds out of %v", dialer.Name(),
			time.Since(dStart).Seconds(), time.Since(start).Seconds())
		// We don't give any special reward for a successful dial here and just rely on
		// the normalized raw throughput to determine the reward. This is because the
		// reward system takes into account how many tries there have been for a given
		// "arm", so giving a reward here would be double-counting.

		// Tell the dialer to update the bandit with it's throughput after 5 seconds.
		dt := newDataTrackingConn(conn)
		time.AfterFunc(secondsForSample*time.Second, func() {
			speed := normalizeReceiveSpeed(dt.dataRecv)
			//log.Debugf("Dialer %v received %v bytes in %v seconds, normalized speed: %v", dialer.Name(), dt.dataRecv, secondsForSample, speed)
			o.bandit.Update(chosenArm, speed)
		})
		o.onSuccess(dialer)
		return dt, err
	}
	o.onFailure()
	return nil, &BanditError{
		attempts: len(o.dialers),
	}
}

// partialDeadline returns the deadline to use for a single address,
// when multiple addresses are pending.
// This is adapated from dial.go in the standard library:
// https://cs.opensource.google/go/go/+/refs/tags/go1.22.0:src/net/dial.go;l=197
func partialDeadline(deadline time.Time, addrsRemaining int) (time.Time, error) {
	now := time.Now()
	if deadline.IsZero() {
		return deadline, nil
	}
	timeRemaining := deadline.Sub(now)
	if timeRemaining <= 0 {
		return time.Time{}, fmt.Errorf("no time remaining")
	}
	// Tentatively allocate equal time to each remaining address.
	timeout := timeRemaining / time.Duration(addrsRemaining)
	// If the time per address is too short, steal from the end of the list.
	const saneMinimum = 2 * time.Second
	if timeout < saneMinimum {
		if timeRemaining < saneMinimum {
			timeout = timeRemaining
		} else {
			timeout = saneMinimum
		}
	}

	// We are not dialing in parallel, so don't let any dialer take up too
	// much time.
	const saneMaximum = 8 * time.Second
	if timeout > saneMaximum {
		timeout = saneMaximum
	}

	return now.Add(timeout), nil
}

// Choose a different arm than the one we already have, if possible.
func differentArm(existingArm, numDialers int, eg *bandit.EpsilonGreedy) int {
	// We let the bandit choose a new arm (or at least try to) versus just rotating to the next
	// dialer because we want to use the bandit's algorithm for optimizing exploration
	// versus exploitation, i.e. it will choose another dialer with a probability
	// proportional to how well it has performed in the past as well as to whether
	// or not we need more data from it. Basically, it will choose a more useful
	// dialer than our random selection.
	if numDialers == 1 {
		log.Debugf("Only one dialer, so returning the existing arm: %v", existingArm)
		return existingArm
	}
	for i := 0; i < 20; i++ {
		newArm := eg.SelectArm(rand.Float64())
		if newArm != existingArm {
			return newArm
		}
	}
	// If we have to choose ourselves, just choose another one.
	return (existingArm + 1) % numDialers
}

func (o *BanditDialer) onSuccess(dialer Dialer) {
	countryCode, country, city := proxyLoc(dialer)
	o.statsTracker.SetActiveProxyLocation(
		city,
		country,
		countryCode,
	)
	o.statsTracker.SetHasSucceedingProxy(true)
}

func (o *BanditDialer) onFailure() {
	o.statsTracker.SetHasSucceedingProxy(false)
}

const secondsForSample = 6

// 1 Mbps in bytes per second is the upper end of speeds we expect to see in such a
// short time period.
const topExpectedBytes = 125000

func normalizeReceiveSpeed(dataRecv uint64) float64 {
	// Return a normalized value between 0 and 1 representing the dailer's
	// bandwidth as a percentage of a theoreticaly upper bound of 200Mbps.
	if dataRecv == 0 {
		return 0
	}
	// Record the bytes in relation to the top expected speed.
	return (float64(dataRecv) / secondsForSample) / topExpectedBytes
}

func (o *BanditDialer) Close() {
	log.Debug("Closing all dialers")
	for _, dialer := range o.dialers {
		dialer.Stop()
	}
}

func newDataTrackingConn(conn net.Conn) *dataTrackingConn {
	return &dataTrackingConn{
		Conn: conn,
	}
}

type dataTrackingConn struct {
	net.Conn
	dataSent uint64
	dataRecv uint64
}

func (c *dataTrackingConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	c.dataSent += uint64(n)
	return n, err
}

func (c *dataTrackingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.dataRecv += uint64(n)
	return n, err
}

// Dialer provides the ability to dial a proxy and obtain information needed to
// effectively load balance between dialers.
type Dialer interface {
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

	WriteStats(w io.Writer)
}
