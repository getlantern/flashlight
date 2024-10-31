package dialer

import (
	"context"
	"errors"
	"io"
	"math"
	"net"
	"time"

	"github.com/getlantern/flashlight/v7/stats"
	"github.com/getlantern/golog"
)

var log = golog.LoggerFor("dialer")

func New(opts *Options) (Dialer, error) {
	return NewConnectivityCheckDialer(opts, func(opts *Options) (Dialer, error) {
		return NewBandit(opts)
	})
}

const (
	// NetworkConnect is a pseudo network name to instruct the dialer to establish
	// a CONNECT tunnel to the proxy.
	NetworkConnect = "connect"
	// NetworkPersistent is a pseudo network name to instruct the dialer to
	// signal the proxy to establish a persistent HTTP connection over which
	// one or more HTTP requests can be sent directly.
	NetworkPersistent = "persistent"
)

// Options are the options used to create a new bandit
type Options struct {
	// The available dialers to use when creating a new bandit
	Dialers []ProxyDialer

	// OnError is the onError callback that is called when bandit encounters a dial error
	OnError func(error, bool)

	// OnSuccess is the callback that is called by bandit after a successful dial.
	OnSuccess func(ProxyDialer)

	// StatsTracker is a stats.Tracker bandit should be configured to use (a callback that is called
	// when a dialer is selected)
	StatsTracker stats.Tracker
}

func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}
	return &Options{
		Dialers:      o.Dialers,
		OnError:      o.OnError,
		OnSuccess:    o.OnSuccess,
		StatsTracker: o.StatsTracker,
	}
}

type Dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

// ProxyDialer provides the ability to dial a proxy and obtain information needed to
// effectively load balance between dialers.
type ProxyDialer interface {

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

type waitForConnectionDialer struct {
	connectedChan chan bool
	next          ProxyDialer
}

func newWaitForConnectionDialer(connectedChan chan bool) *connectTimeProxyDialer {
	return &connectTimeProxyDialer{
		ProxyDialer: &waitForConnectionDialer{
			connectedChan: connectedChan,
		},
		connectTime: time.Duration(math.MaxInt64),
	}
}

func (n *waitForConnectionDialer) DialProxy(ctx context.Context) (net.Conn, error) {
	return nil, errors.New("noOpDialer cannot dial proxy")
}

func (n *waitForConnectionDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, bool, error) {
	select {
	// Exit if the context times out.
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case <-n.connectedChan:
		return n.next.DialContext(ctx, network, addr)
	}
}

func (n *waitForConnectionDialer) Name() string {
	return "noop"
}

func (n *waitForConnectionDialer) Addr() string {
	return "noop"
}

func (n *waitForConnectionDialer) Label() string {
	return "noop"
}

func (n *waitForConnectionDialer) Attempts() int64 {
	return 0
}

func (n *waitForConnectionDialer) ConsecFailures() int64 {
	return 0
}

func (n *waitForConnectionDialer) ConsecSuccesses() int64 {
	return 0
}

func (n *waitForConnectionDialer) DataRecv() uint64 {
	return 0
}

func (n *waitForConnectionDialer) DataSent() uint64 {
	return 0
}

func (n *waitForConnectionDialer) FormatStats() []string {
	return nil
}

func (n *waitForConnectionDialer) DialProxyWithTimeout(ctx context.Context, timeout time.Duration) (net.Conn, error) {
	return nil, nil
}

func (n *waitForConnectionDialer) DialProxyWithTimeoutAndAddr(ctx context.Context, timeout time.Duration, addr string) (net.Conn, error) {
	return nil, nil
}

func (n *waitForConnectionDialer) DialProxyWithAddr(ctx context.Context, addr string) (net.Conn, error) {
	return nil, nil
}

func (n *waitForConnectionDialer) DialProxyWithTimeoutAndNetwork(ctx context.Context, timeout time.Duration, network string) (net.Conn, error) {
	return nil, nil
}

func (n *waitForConnectionDialer) DialProxyWithNetwork(ctx context.Context, network string) (net.Conn, error) {
	return nil, nil
}

func (n *waitForConnectionDialer) DialProxyWithTimeoutNetworkAndAddr(ctx context.Context, timeout time.Duration, network, addr string) (net.Conn, error) {
	return nil, nil
}

// EstBandwidth implements ProxyDialer.
func (n *waitForConnectionDialer) EstBandwidth() float64 {
	return 0
}

// EstRTT implements ProxyDialer.
func (n *waitForConnectionDialer) EstRTT() time.Duration {
	return 0
}

// EstSuccessRate implements ProxyDialer.
func (n *waitForConnectionDialer) EstSuccessRate() float64 {
	return 0
}

// Failures implements ProxyDialer.
func (n *waitForConnectionDialer) Failures() int64 {
	return 0
}

// JustifiedLabel implements ProxyDialer.
func (n *waitForConnectionDialer) JustifiedLabel() string {
	return ""
}

// Location implements ProxyDialer.
func (n *waitForConnectionDialer) Location() (string, string, string) {
	return "", "", ""
}

// MarkFailure implements ProxyDialer.
func (n *waitForConnectionDialer) MarkFailure() {
}

// NumPreconnected implements ProxyDialer.
func (n *waitForConnectionDialer) NumPreconnected() int {
	return 0
}

// NumPreconnecting implements ProxyDialer.
func (n *waitForConnectionDialer) NumPreconnecting() int {
	return 0
}

// Protocol implements ProxyDialer.
func (n *waitForConnectionDialer) Protocol() string {
	return "noop"
}

// Stop implements ProxyDialer.
func (n *waitForConnectionDialer) Stop() {
}

// Succeeding implements ProxyDialer.
func (n *waitForConnectionDialer) Succeeding() bool {
	return false
}

// Successes implements ProxyDialer.
func (n *waitForConnectionDialer) Successes() int64 {
	return 0
}

// SupportsAddr implements ProxyDialer.
func (n *waitForConnectionDialer) SupportsAddr(network string, addr string) bool {
	return false
}

// Trusted implements ProxyDialer.
func (n *waitForConnectionDialer) Trusted() bool {
	return false
}

// WriteStats implements ProxyDialer.
func (n *waitForConnectionDialer) WriteStats(w io.Writer) {
}
