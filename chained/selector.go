package chained

import (
	"context"
	"fmt"
	"io"
	"iter"
	"net"
	"sync/atomic"
	"time"
)

const (
	// NetworkConnect is a pseudo network name to instruct the dialer to establish
	// a CONNECT tunnel to the proxy.
	NetworkConnect = "connect"
	// NetworkPersistent is a pseudo network name to instruct the dialer to
	// signal the proxy to establish a persistent HTTP connection over which
	// one or more HTTP requests can be sent directly.
	NetworkPersistent = "persistent"

	// maxDialerFails is the max consecutive failed dials/requests before moving to the next dialer
	maxDialerFails = 4
)

// Dialer provides the ability to dial a proxy
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

type Selector struct {
	dialers []Dialer
	// active is the index of the currently active dialer
	active atomic.Int64
}

func NewSelector(dialers []Dialer) *Selector {
	return &Selector{dialers: dialers}
}

func (s *Selector) Dial(network, addr string) (net.Conn, error) {
	return s.DialContext(context.Background(), network, addr)
}

// DialContext dials the given network and address. It will cycle through the available dialers
// until one is successful, or all have been attempted.
func (s *Selector) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if len(s.dialers) == 0 {
		return nil, log.Error("no dialers available")
	}

	for i, dlr := range s.iter() {
		if !dlr.SupportsAddr(network, addr) {
			log.Debugf("dialer %s does not support %s %s", dlr.Label(), network, addr)
			continue
		}

		log.Debugf("dialing %s %s with dialer %s", network, addr, dlr.Label())
		conn, err := s.dialWithDialer(ctx, dlr, network, addr)
		if err == nil {
			log.Debugf("dialer %s successfully dialed %s %s", dlr.Label(), network, addr)
			s.active.Store(int64(i))
			return conn, nil
		}

		select {
		case <-ctx.Done():
			err := fmt.Errorf("dialing %s %s: %v", network, addr, ctx.Err())
			log.Error(err)
			return nil, err
		default:
		}

		log.Errorf("dialer %s failed to dial %s %s: %v", dlr.Label(), network, addr, err)
	}

	return nil, fmt.Errorf("failed to dial %s %s", network, addr)
}

func (s *Selector) dialWithDialer(ctx context.Context, dlr Dialer, network, addr string) (net.Conn, error) {
	for {
		conn, failedUpstream, err := dlr.DialContext(ctx, network, addr)
		if err == nil {
			return conn, nil
		}

		select {
		case <-ctx.Done():
			log.Debugf("dialWithDialer: %v", ctx.Err())
			return nil, err
		default:
		}

		dlr.MarkFailure()
		if dlr.ConsecFailures() >= maxDialerFails {
			if failedUpstream {
				err = fmt.Errorf("failed upstream: %w", err)
			}

			return nil, err
		}
	}
}

// MarkFailure marks the active dialer as failed. This is used to provide feedback to the Selector
// that requests are failing. This could be due poor proxy performance, or the proxy being blocked.
func (s *Selector) MarkFailure() {
	// BUG: There is an edge case where the active dialer (proxy) was used successfully by 2+
	// goroutines and before failing. In this case, all goroutines will mark the dialer as failed.
	// If the dialer advances before the other goroutines mark it as failed, then the new active
	// dialer will be marked as failed incorrectly.
	dlr := s.dialers[s.active.Load()]
	dlr.MarkFailure()
}

// iter returns a function that iterates through the dialers starting from the active dialer and
// wrapping around to the beginning. This for use in a range-over-function loop. It will yield the
// next dialer in the list and its index in s.dialers.
func (s *Selector) iter() iter.Seq2[int, Dialer] {
	i := int(s.active.Load())
	return func(yield func(int, Dialer) bool) {
		for j := 0; j < len(s.dialers); j++ {
			dlr := s.dialers[i]
			if !yield(i, dlr) {
				return
			}

			i = (i + 1) % len(s.dialers)
		}
	}
}
