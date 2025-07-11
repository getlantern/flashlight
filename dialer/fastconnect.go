package dialer

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"
)

type connectTimeProxyDialer struct {
	ProxyDialer

	connectTime time.Duration
}

type connectedDialers struct {
	dialers []connectTimeProxyDialer
	sync.RWMutex
}

// fastConnectDialer stores the time it took to connect to each dialer and uses
// that information to select the fastest dialer to use.
type fastConnectDialer struct {
	topDialer protectedDialer
	connected connectedDialers

	next func(*Options)
	opts *Options

	// Create a channel for stopping connections to dialers
	stopCh chan struct{}
}

// Make sure fastConnectDialer implements Dialer
var _ Dialer = (*fastConnectDialer)(nil)

func newFastConnectDialer(opts *Options) *fastConnectDialer {
	if opts.OnError == nil {
		opts.OnError = func(error, bool) {}
	}
	if opts.OnSuccess == nil {
		opts.OnSuccess = func(ProxyDialer) {}
	}
	fcd := &fastConnectDialer{
		connected: connectedDialers{
			dialers: make([]connectTimeProxyDialer, 0),
		},
		opts: opts,
		next: func(opts *Options) {
			banditDialer, err := NewBandit(opts)
			if err != nil {
				log.Errorf("Unable to create bandit: %v", err)
			} else {
				if opts.OnNewDialer != nil {
					log.Debug("Switching to bandit dialer")
					opts.OnNewDialer(newParallelPreferProxyless(opts.proxylessDialer, banditDialer, opts))
				} else {
					log.Errorf("No onNewDialer function set -- should never happen")
				}
			}
		},
		topDialer: protectedDialer{},
		stopCh:    make(chan struct{}, 10),
	}
	go fcd.connectAll(opts.Dialers)
	return fcd
}

func (fcd *fastConnectDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// Use the dialer with the lowest connect time, waiting on early dials for any
	// connections at all.
	td := fcd.topDialer.get()
	if td == nil {
		return nil, fmt.Errorf("no top dialer")
	}

	// Note that we don't currently check if the dialer supports
	// the domain here.
	conn, failedUpstream, err := td.DialContext(ctx, network, addr)
	if err != nil {
		hasSucceeding := len(fcd.connected.dialers) > 0
		fcd.opts.OnError(err, hasSucceeding)
		// Error connecting to the proxy or to the destination
		if failedUpstream {
			// Error connecting to the destination
			log.Debugf("Error connecting to upstream destination %v: %v", addr, err)
		} else {
			// Error connecting to the proxy
			log.Debugf("Error connecting to proxy %v: %v", td.Name(), err)
		}
		return nil, err
	}
	fcd.opts.OnSuccess(td)
	return conn, err
}

func (fcd *fastConnectDialer) Close() {
	// We don't call Stop on the Dialers themselves here because they are likely
	// in use by other Dialers, such as the BanditDialer.
	// Stop all dialing
	fcd.stopCh <- struct{}{}
}

// OnOptions is called when the options change. We need to stop all dialing and restart
// from the initial state.
//
// Note that in practice, at least as of this writing, this should never
// be called because it should always be "shielded" by the proxyless dialer
// within the parallel dialer, and the parallel dialer will be notified of
// the new options.
func (fcd *fastConnectDialer) OnOptions(opts *Options) Dialer {
	log.Errorf("OnOptions called on fastConnectDialer -- should never happen")
	// When we get new dialers, we need to:
	// 1. Stop existing connectivity checks
	// 2. Start new connectivity checks
	fcd.Close()
	return opts.proxylessDialer.OnOptions(opts)
}

func (fcd *fastConnectDialer) onConnected(pd ProxyDialer, connectTime time.Duration) {
	log.Debugf("Connected to %v", pd.Name())

	newTopDialer := fcd.connected.onConnected(pd, connectTime)

	// Set top dialer if the fastest dialer changed.
	td := fcd.topDialer.get()
	if td != newTopDialer {
		log.Debugf("Setting new top dialer to %v", newTopDialer.Name())
		fcd.topDialer.set(newTopDialer)
	}
	fcd.opts.OnSuccess(fcd.topDialer.get())
}

// connectAll dials all the dialers in parallel to connect the user as quickly as
// possible on startup.
func (fcd *fastConnectDialer) connectAll(dialers []ProxyDialer) {
	if len(dialers) == 0 {
		log.Errorf("No dialers to connect to")
		return
	}
	log.Debugf("Dialing all dialers in parallel %#v", dialers)
	for {
		// Loop until we're connected
		if len(fcd.connected.dialers) < 2 {
			fcd.parallelDial(dialers)
		} else {
			break
		}
		select {
		case <-fcd.stopCh:
			log.Debug("Stopping parallel dialing")
			return
		case <-time.After(time.Duration(rand.Intn(4000)) * time.Millisecond):
		}
	}
	// At this point, we've tried all of the dialers, and they've all either
	// succeeded or failed.

	// If we've connected to more than one dialer after trying all of them,
	// switch to the next dialer that's optimized for multiple connections.
	nextOpts := fcd.opts.Clone()
	nextOpts.Dialers = fcd.connected.proxyDialers()
	fcd.next(nextOpts)
}

func (fcd *fastConnectDialer) parallelDial(dialers []ProxyDialer) {
	log.Debug("Connecting to all dialers")
	var wg sync.WaitGroup
	for _, d := range dialers {
		wg.Add(1)
		go func(pd ProxyDialer) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer func() {
				log.Debugf("fastConnectDialer::parallelDial::canceling context for %v", pd.Name())
				cancel()
			}()
			start := time.Now()
			conn, err := pd.DialProxy(ctx)
			defer func() {
				if conn != nil {
					conn.Close()
				}
			}()
			if err != nil {
				log.Debugf("Dialer %v failed in %v with: %v", pd.Name(), time.Since(start), err)
				return
			}

			log.Debugf("Dialer %v succeeded in %v", pd.Name(), time.Since(start))
			fcd.onConnected(pd, time.Since(start))
		}(d)
	}
	wg.Wait()
}

// Accessor for a copy of the ProxyDialer slice
func (cd *connectedDialers) proxyDialers() []ProxyDialer {
	cd.RLock()
	defer cd.RUnlock()

	dialers := make([]ProxyDialer, len(cd.dialers))

	// Note that we manually copy here vs using copy because we need an array of
	// ProxyDialers, not a dialersByConnectTime.
	for i, ctd := range cd.dialers {
		dialers[i] = ctd.ProxyDialer
	}
	return dialers
}

// onConnected adds a connected dialer to the list of connected dialers and returns
// the fastest dialer.
func (cd *connectedDialers) onConnected(pd ProxyDialer, connectTime time.Duration) ProxyDialer {
	cd.Lock()
	defer cd.Unlock()

	cd.dialers = append(cd.dialers, connectTimeProxyDialer{
		ProxyDialer: pd,
		connectTime: connectTime,
	})
	sort.Slice(cd.dialers, func(i, j int) bool {
		return cd.dialers[i].connectTime < cd.dialers[j].connectTime
	})
	return cd.dialers[0].ProxyDialer
}

// protectedDialer protects a dialer.Dialer with a RWMutex. We can't use an atomic.Value here
// because ProxyDialer is an interface.
type protectedDialer struct {
	sync.RWMutex
	dialer ProxyDialer
}

// set sets the dialer in the protectedDialer
func (pd *protectedDialer) set(dialer ProxyDialer) {
	pd.Lock()
	defer pd.Unlock()
	pd.dialer = dialer
}

// get gets the dialer from the protectedDialer
func (pd *protectedDialer) get() ProxyDialer {
	pd.RLock()
	defer pd.RUnlock()
	return pd.dialer
}
