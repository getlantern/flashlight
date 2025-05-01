package dialer

import (
	"context"
	"errors"
	"net"
)

type parallelDialer struct {
	proxylessDialer proxyless
	dialer          Dialer
	opts            *Options
}

func newParallelPreferProxyless(proxyless proxyless, d Dialer, opts *Options) Dialer {
	return &parallelDialer{
		proxylessDialer: proxyless,
		dialer:          d,
		opts:            opts,
	}
}

func (d *parallelDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	switch d.proxylessDialer.status(addr) {
	case SUCCEEDED:
		log.Debugf("Proxyless dialer succeeded for %s, using it", addr)
		// If the proxyless dialer is succeeding, keep using it.
		return d.proxylessDialer.DialContext(ctx, network, addr)
	case FAILED:
		// If the proxyless dialer has failed, we fall back to the default dialer.
		log.Debugf("Proxyless dialer failed for %s, falling back to default dialer", addr)
		return d.dialer.DialContext(ctx, network, addr)
	case UNKNOWN:
		// Fallthrough
	}
	log.Debugf("Proxyless dialer status is unknown for %s, trying both dialers", addr)

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
	for numErrs < len(dialers) {
		select {
		case conn := <-connCh:
			// Return the first successful connection immediately.
			return conn, nil
		case err := <-errCh:
			errs = errors.Join(errs, err)
			numErrs++
		case <-ctx.Done():
			log.Debugf("parallelDialer::DialContext::context done: %v", ctx.Err())
			return nil, ctx.Err()
		}
	}
	// If all dialers failed, reset the proxyless dialer and return the aggregated error.
	d.proxylessDialer.reset(addr)
	return nil, errs
}

func (d *parallelDialer) Close() {
	d.dialer.Close()
}

// OnOptions notifies the dialer of new Options.
// This is called when the options change, and we need to stop all dialing
// and restart from the initial state.
func (d *parallelDialer) OnOptions(opts *Options) Dialer {
	log.Debugf("OnOptions called on parallelDialer with %v dialers", len(opts.Dialers))
	// Any time there are new options, we need to go back to the beginning and use
	// whatever the proxyless dialer does when it gets new options. We also need
	// to close the backup, non-proxyless dialer.
	d.dialer.Close()
	return d.proxylessDialer.OnOptions(opts)
}
