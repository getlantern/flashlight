package orchestrator

import (
	"context"
	"io"
	"math/rand"
	"net"
	"time"

	bandit "github.com/alextanhongpin/go-bandit"
	"github.com/getlantern/golog"
)

var log = golog.LoggerFor("orchestrator")

// Orchestrator is responsible for continually choosting the optimized
// dialer.
type Orchestrator struct {
	dialers []Dialer
	bandit  *bandit.EpsilonGreedy
}

// New creates a new Orchestrator given the available dialers.
func New(dialers []Dialer) (*Orchestrator, error) {
	b, err := bandit.NewEpsilonGreedy(0.1, nil, nil)
	if err != nil {
		log.Errorf("Unable to create bandit: %v", err)
		return nil, err
	}

	b.Init(len(dialers))

	return &Orchestrator{
		dialers: dialers,
		bandit:  b,
	}, nil
}

func (o *Orchestrator) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	chosenArm := o.bandit.SelectArm(rand.Float64())

	// We have to be careful here about virtual, multiplexed connections, as the
	// initial TCP dial will have different performance characteristics than the
	// subsequent virtual connection dials.
	for i := 0; i < len(o.dialers); i++ {
		dialer := o.dialers[chosenArm]
		conn, failedUpstream, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			log.Errorf("dialer %d failed: %v", chosenArm, err)
			if !failedUpstream {
				o.bandit.Update(chosenArm, 0)
			}
			continue
		}
		// Tell the dialer to update the bandit with it's throughput after 5 seconds.
		dt := newDataTrackingConn(conn)

		// Feels like this could be optimized more, but I'm not sure how.
		time.AfterFunc(5*time.Second, func() {
			o.bandit.Update(chosenArm, float64(dt.dataRecv)/5)
		})
		return dt, err
	}
	return nil, log.Errorf("all dialers failed")
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
