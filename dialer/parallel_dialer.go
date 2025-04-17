package dialer

import (
	"context"
	"errors"
	"fmt"
	"net"
)

type parallelDialer struct {
	proxylessDialer proxyless
	dialer          Dialer
}

func newParallelPreferProxyless(proxyless proxyless, d Dialer) Dialer {
	return &parallelDialer{
		proxylessDialer: proxyless,
		dialer:          d,
	}
}

func (d *parallelDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	switch d.proxylessDialer.status(addr) {
	case SUCCEEDED:
		// If the proxyless dialer is succeeding, keep using it.
		return d.proxylessDialer.DialContext(ctx, network, addr)
	case FAILED:
		// If the proxyless dialer has failed, we fall back to the default dialer.
		return d.dialer.DialContext(ctx, network, addr)
	case UNKNOWN:
		// Fallthrough
	}

	// If we the status of the proxyless dialer is undetermined, we try both.
	// This could be the case when we haven't tried the proxyless dialer yet,
	// but it could also be the case if both are failing. The latter could
	// happen, for example, if the user has lost their internet connection.
	var errs error
	var connCh = make(chan net.Conn, 2)
	var errCh = make(chan error, 2)
	dialers := []Dialer{d.proxylessDialer, d.dialer}
	for _, d := range dialers {
		go func(d Dialer) {
			conn, err := d.DialContext(ctx, network, addr)
			if err != nil {
				errCh <- err
			} else {
				connCh <- conn
			}
		}(d)
	}

	numErrs := 0
	select {
	case conn := <-connCh:
		return conn, nil
	case err := <-errCh:
		errs = errors.Join(err)
		numErrs++
		if numErrs == len(dialers) {
			// If both dialers failed, it could be that we don't have a network
			// connection, for example, so we reset the proxyless dialer to its
			// state for this domain such that we will try it again.
			d.proxylessDialer.reset(addr)
			return nil, errs
		}
	case <-ctx.Done():
		log.Debugf("parallelDialer::DialContext::context done: %v", ctx.Err())
		return nil, ctx.Err()
	}

	// Should not be reachable
	return nil, fmt.Errorf("Could not dial any dialers")
}

func (d *parallelDialer) Close() {
	d.dialer.Close()
}

func (d *parallelDialer) OnOptions(opts *Options) Dialer {
	return d.dialer.OnOptions(opts)
}
