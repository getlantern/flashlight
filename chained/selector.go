package chained

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

// selector
// 	is dialer
// 	dialwithdialer
// 	stores proxy dialers
// 	feedback: worked/failed
// 	if failed x times, use next proxy
// 	reset?
//
// check if dialer supports network/address when picking
// log stats?

// Dialer provides the ability to dial a proxy
type Dialer interface {
	// DialProxy dials the proxy but does not yet dial the origin.
	DialProxy(ctx context.Context) (net.Conn, error)
	// DialContext dials out to the given origin. failedUpstream indicates whether
	// this was an upstream error (as opposed to errors connecting to the proxy).
	DialContext(ctx context.Context, network, addr string) (conn net.Conn, err error)

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
	// SupportsAddr indicates whether this Dialer supports the given addr. If it does not, the
	// balancer will not attempt to dial that addr with this Dialer.
	SupportsAddr(network, addr string) bool

	// MarkFailure marks a dial failure on this dialer.
	MarkFailure()
	// Attempts returns the total number of dial attempts
	Attempts() int
	// Successes returns the total number of dial successes
	Successes() int
	// Failures returns the total number of dial failures
	Failures() int
	// ConsecFailures returns the number of consecutive dial failures
	ConsecFailures() int

	// Stop stops background processing for this Dialer.
	Stop()

	WriteStats(w io.Writer)
}

type Selector struct {
	dialers []Dialer
	// active is the index of the currently active dialer
	active atomic.Int64
	// maxDialerFails is the max consecutive failed dials/requests before moving to the next dialer
	maxDialerFails int

	advanceMu sync.Mutex
}

func NewSelector(dialers []Dialer, maxDialerFails int) *Selector {
	return &Selector{
		dialers:        dialers,
		maxDialerFails: maxDialerFails,
	}
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

	first := s.active.Load()
	for {
		select {
		case <-ctx.Done():
			err := fmt.Errorf("dialing %s %s: %v", network, addr, ctx.Err())
			log.Error(err)
			return nil, err
		default:
		}

		dlr := s.dialers[s.active.Load()]
		if !dlr.SupportsAddr(network, addr) {
			log.Debugf("dialer %s does not support %s %s", dlr.Label(), network, addr)
			continue
		}

		conn, err := dlr.DialContext(ctx, network, addr)
		if err == nil {
			return conn, nil
		}

		log.Errorf("dialer %s failed to dial %s %s: %v", dlr.Label(), network, addr, err)
		s.MarkFailure()
		if s.active.Load() >= first {
			// we've tried all dialers and failed to connect
			return nil, fmt.Errorf("failed to dial %s %s", network, addr)
		}
	}
}

// MarkFailure marks the active dialer as failed. This is used to provide feedback to the Selector
// that requests are failing. This could be due poor proxy performance, or the proxy being blocked.
func (s *Selector) MarkFailure() {
	// BUG: there is an edge case where the active dialer (proxy) was used successfully by 2+
	// goroutines and then fails. In this case, all goroutines will mark the dialer as failed.
	// If the dialer advances before the other goroutines mark it as failed, then the new active
	// dialer will be marked as failed incorrectly.
	s.advanceMu.Lock()
	defer s.advanceMu.Unlock()

	d := s.active.Load()
	s.dialers[d].MarkFailure()

	if s.shouldAdvance() {
		d++
		if d >= int64(len(s.dialers)) {
			d = 0
		}

		s.active.Store(d)
	}
}

func (s *Selector) shouldAdvance() bool {
	return s.dialers[s.active.Load()].ConsecFailures() >= s.maxDialerFails
}
