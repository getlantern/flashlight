package chained

import (
	"context"
	"fmt"
	"io"
	"iter"
	"net"
	"sync/atomic"
)

const (
	// NetworkConnect is a pseudo network name to instruct the dialer to establish
	// a CONNECT tunnel to the proxy.
	NetworkConnect = "connect"
	// NetworkPersistent is a pseudo network name to instruct the dialer to
	// signal the proxy to establish a persistent HTTP connection over which
	// one or more HTTP requests can be sent directly.
	NetworkPersistent = "persistent"
)

// Dialer provides the ability to dial a proxy
type Dialer interface {
	// SupportsAddr indicates whether this Dialer supports the given addr. If it does not, the
	// balancer will not attempt to dial that addr with this Dialer.
	SupportsAddr(network, addr string) bool
	// DialContext dials out to the given origin. failedUpstream indicates whether
	// this was an upstream error (as opposed to errors connecting to the proxy).
	DialContext(ctx context.Context, network, addr string) (conn net.Conn, failedUpstream bool, err error)
	// Label returns a label for this Dialer (includes Name plus more).
	Label() string
	// MarkFailure marks a dial failure on this dialer.
	MarkFailure()
	// Stop stops background processing for this Dialer.
	Stop()
	// WriteStats writes this Dialer's stats to the given writer.
	WriteStats(w io.Writer)
}

type LoopDialer struct {
	dialers []Dialer
	// active is the index of the currently active dialer
	active atomic.Int64
}

// TODO: Add a way for LoopDialer to accept new Dialers. The new Dialers should replace the old ones
// and if the active dialer exists in the new set, it should remain the active dialer. Otherwise,
// set the active dialer to the first dialer in the new set.

func NewLoopDialer(dialers []Dialer) *LoopDialer {
	return &LoopDialer{dialers: dialers}
}

// Close stops background processing for all Dialers in this LoopDialer.
func (ld *LoopDialer) Close() {
	for _, dlr := range ld.dialers {
		dlr.Stop()
	}
}

func (ld *LoopDialer) Dial(network, addr string) (net.Conn, error) {
	return ld.DialContext(context.Background(), network, addr)
}

// DialContext dials the given network and address. It will cycle through the available dialers
// until one is successful, or all have been attempted.
func (ld *LoopDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if len(ld.dialers) == 0 {
		return nil, log.Error("no dialers available")
	}

	for i, dlr := range ld.iter() {
		select {
		case <-ctx.Done():
			err := fmt.Errorf("dialing %s %s: %w", network, addr, ctx.Err())
			log.Error(err)
			return nil, err
		default:
		}

		if !dlr.SupportsAddr(network, addr) {
			log.Debugf("dialer %s does not support %s %s", dlr.Label(), network, addr)
			continue
		}

		log.Debugf("dialing %s %s with dialer %s", network, addr, dlr.Label())
		conn, err := ld.dialWithDialer(ctx, dlr, network, addr)
		if err == nil {
			log.Debugf("dialer %s successfully dialed %s %s", dlr.Label(), network, addr)
			ld.active.Store(int64(i))
			return conn, nil
		}

		log.Errorf("dialer %s failed to dial %s %s: %v", dlr.Label(), network, addr, err)
	}

	return nil, fmt.Errorf("failed to dial %s %s", network, addr)
}

func (ld *LoopDialer) dialWithDialer(ctx context.Context, dlr Dialer, network, addr string) (net.Conn, error) {
	conn, failedUpstream, err := dlr.DialContext(ctx, network, addr)
	if err == nil {
		return conn, nil
	}

	if failedUpstream {
		err = fmt.Errorf("failed upstream: %w", err)
		return nil, err
	}

	dlr.MarkFailure()
	return nil, err
}

// iter returns a function that iterates through the dialers starting from the active dialer and
// wrapping around to the beginning. This for use in a range-over-function loop. It will yield the
// next dialer in the list and its index in s.dialers.
func (ld *LoopDialer) iter() iter.Seq2[int, Dialer] {
	i := int(ld.active.Load())
	return func(yield func(int, Dialer) bool) {
		for j := 0; j < len(ld.dialers); j++ {
			dlr := ld.dialers[i]
			if !yield(i, dlr) {
				return
			}

			i = (i + 1) % len(ld.dialers)
		}
	}
}
