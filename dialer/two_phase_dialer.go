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
// the dialers that can connect at all.
type twoPhaseDialer struct {
	activeDialer     Dialer
	activeDialerLock sync.RWMutex
}

// TwoPhaseDialer creates a new dialer for checking proxy connectivity.
func TwoPhaseDialer(opts *Options, next func(opts *Options, existing Dialer) Dialer) Dialer {
	log.Debugf("Creating new fast dialer with %d dialers", len(opts.Dialers))

	tpd := &twoPhaseDialer{}

	fcd := newFastConnectDialer(opts, func(dialerOpts *Options, existing Dialer) Dialer {
		// This is where we move to the second dialer.
		nextDialer := next(dialerOpts, existing)
		tpd.storeActiveDialer(nextDialer)
		return nextDialer
	})

	tpd.storeActiveDialer(fcd)

	fcd.connectAll(opts.Dialers)

	return tpd
}

// DialContext implements Dialer.
func (ccd *twoPhaseDialer) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	td := ccd.loadActiveDialer()
	if td == nil {
		return nil, errors.New("no active dialer")
	}
	return td.DialContext(ctx, network, addr)
}

func (fcd *twoPhaseDialer) storeActiveDialer(active Dialer) {
	fcd.activeDialerLock.Lock()
	defer fcd.activeDialerLock.Unlock()
	fcd.activeDialer = active
}

func (fcd *twoPhaseDialer) loadActiveDialer() Dialer {
	fcd.activeDialerLock.RLock()
	defer fcd.activeDialerLock.RUnlock()
	return fcd.activeDialer
}
