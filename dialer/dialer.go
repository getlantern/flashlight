// Package dialer contains the interfaces for creating connections to proxies. It is designed
// to first connect as quickly as possible, and then to optimize for bandwidth and latency
// based on the proxies that are accessible. It does this by:
//
// 1) First connecting using proxyless strategies when we have no proxies
// 2) As soon as we have proxies, using a combination of proxyless and proxied while testing
// the proxies for connectivity and latency, using connection time as a proxy for latency.
// 3) When we know what proxies are connecting, switching to a combination of proxyless and
// proxies based on which proxies are performing best (multi-armed bandit)
package dialer

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/getlantern/golog"
)

var log = golog.LoggerFor("dialer")

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
	// The available dialers to use when creating a new dialer
	Dialers []ProxyDialer

	// OnError is the onError callback that is called when the dialer encounters a dial error
	OnError func(error, bool)

	// OnSuccess is the callback that is called by dialer after a successful dial.
	OnSuccess func(ProxyDialer)

	// BanditDir is the directory where the bandit will store its data
	BanditDir string

	proxylessDialer proxyless

	OnNewDialer func(Dialer)
}

// Clone creates a deep copy of the Options object
func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}
	return &Options{
		Dialers:         o.Dialers,
		OnError:         o.OnError,
		OnSuccess:       o.OnSuccess,
		BanditDir:       o.BanditDir,
		proxylessDialer: o.proxylessDialer,
		OnNewDialer:     o.OnNewDialer,
	}
}

// Dialer is the interface for creating connections destination sites. This is sent over a proxy connection,
// with the proxy handling the actual connection to the destination site. This represents a "Dialer" at the
// highest level in the sense that a Dialer is not tied to any specific proxy, but rather represents a higher
// level abstraction that can be used to dial any proxy depending on optimization strategies.
type Dialer interface {

	// DialContext dials out to the domain or IP address representing a destination site.
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)

	// Close closes the dialer and cleans up any resources
	Close()

	// OnOptions notifies the dialer of new Options.
	OnOptions(opts *Options) Dialer
}

// hasSucceedingDialer checks whether or not any of the given dialers is able to successfully dial our proxies
func hasSucceedingDialer(dialers []ProxyDialer) bool {
	for _, d := range dialers {
		if d.ConsecFailures() == 0 && d.Successes() > 0 {
			return true
		}
	}
	return false
}

// hasNotFailing checks whether or not any of the given dialers are not explicitly failing
func hasNotFailing(dialers []ProxyDialer) bool {
	for _, d := range dialers {
		if d.ConsecFailures() == 0 {
			return true
		}
	}
	return false
}

// ProxyDialer provides the ability to dial a proxy and obtain information needed to
// record performance data about proxies.
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

	// Ready returns a channel which will have a value on it only when initialization
	// of the dialer is complete. If initialization failed, the channel will have a non-nil
	// error value. If initialization is not required, this will return nil channel value.
	Ready() <-chan error

	// Stop stops background processing for this Dialer.
	Stop()

	WriteStats(w io.Writer)
}
