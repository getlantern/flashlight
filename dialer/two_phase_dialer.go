package dialer

import (
	"context"
	"errors"
	"net"
	"sync"
)

// twoPhaseDialer implements a two-phase approach to dialing. First, it tries to
// connect as quickly as possilbe to get the user online as soon as possible while also
// determining which dialers are able to connect at all. It then switches to a
// multi-armed bandit based dialer that tries to find the fastest dialer amongst all
// the dialers that can connect.
type twoPhaseDialer struct {
	activeDialer activeDialer
}

// Make sure twoPhaseDialer implements Dialer
var _ Dialer = (*twoPhaseDialer)(nil)

// NewTwoPhaseDialer creates a new dialer for checking proxy connectivity.
func NewTwoPhaseDialer(opts *Options, next func(opts *Options, existing Dialer) Dialer) Dialer {
	log.Debugf("Creating new two phase dialer with %d dialers", len(opts.Dialers))

	tpd := &twoPhaseDialer{}

	fcd := newFastConnectDialer(opts, func(dialerOpts *Options, existing Dialer) Dialer {
		// This is where we move to the second dialer.
		nextDialer := next(dialerOpts, existing)
		tpd.activeDialer.set(nextDialer)
		return nextDialer
	})

	tpd.activeDialer.set(fcd)

	go fcd.connectAll(opts.Dialers)

	return tpd
}

// DialContext implements Dialer.
func (ccd *twoPhaseDialer) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	td := ccd.activeDialer.get()
	if td == nil {
		return nil, errors.New("no active dialer")
	}
	return td.DialContext(ctx, network, addr)
}

// Close implements Dialer.
func (ccd *twoPhaseDialer) Close() {
	td := ccd.activeDialer.get()
	if td != nil {
		td.Close()
	}
}

// protectedDialer protects a dialer.Dialer with a RWMutex. We can't use an atomic.Value here
// because Dialer is an interface.
type activeDialer struct {
	sync.RWMutex
	dialer Dialer
}

// set sets the dialer in the activeDialer
func (ad *activeDialer) set(dialer Dialer) {
	ad.Lock()
	defer ad.Unlock()
	ad.dialer = dialer
}

// get gets the dialer from the activeDialer
func (ad *activeDialer) get() Dialer {
	ad.RLock()
	defer ad.RUnlock()
	return ad.dialer
}
