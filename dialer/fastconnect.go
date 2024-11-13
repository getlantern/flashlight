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

type dialersByConnectTime []connectTimeProxyDialer

func (d dialersByConnectTime) Len() int {
	return len(d)
}

func (d dialersByConnectTime) Less(i, j int) bool {
	return d[i].connectTime < d[j].connectTime
}

func (d dialersByConnectTime) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}

// fastConnectDialer stores the time it took to connect to each dialer and uses
// that information to select the fastest dialer to use.
type fastConnectDialer struct {
	topDialer     protectedDialer
	connected     dialersByConnectTime
	connectedChan chan int
	// Lock for the slice of dialers.
	connectedLock sync.RWMutex

	next func(*Options, Dialer) Dialer
	opts *Options

	onError   func(error, bool)
	onSuccess func(ProxyDialer)
}

func newFastConnectDialer(opts *Options, next func(opts *Options, existing Dialer) Dialer) *fastConnectDialer {
	if opts.OnError == nil {
		opts.OnError = func(error, bool) {}
	}
	if opts.OnSuccess == nil {
		opts.OnSuccess = func(ProxyDialer) {}
	}
	return &fastConnectDialer{
		connected:     make(dialersByConnectTime, 0),
		connectedChan: make(chan int),
		opts:          opts,
		next:          next,
		onError:       opts.OnError,
		onSuccess:     opts.OnSuccess,
	}
}

func (fcd *fastConnectDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// Use the dialer with the lowest connect time, waiting on early dials for any
	// connections at all.
	td := fcd.topDialer.get()
	if td == nil {
		return nil, fmt.Errorf("no top dialer")
	}
	conn, failedUpstream, err := td.DialContext(ctx, network, addr)
	if err != nil {
		hasSucceeding := len(fcd.connected) > 0
		fcd.onError(err, hasSucceeding)
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
	fcd.onSuccess(td)
	return conn, err
}

// Accessor for a copy of the ProxyDialer slice
func (fcd *fastConnectDialer) proxyDialers() []ProxyDialer {
	fcd.connectedLock.RLock()
	defer fcd.connectedLock.RUnlock()

	dialers := make([]ProxyDialer, len(fcd.connected))

	// Note that we manually copy here vs using copy because we need an array of
	// ProxyDialers, not a dialersByConnectTime.
	for i, ctd := range fcd.connected {
		dialers[i] = ctd.ProxyDialer
	}
	return dialers
}

func (fcd *fastConnectDialer) onConnected(pd ProxyDialer, connectTime time.Duration) {
	log.Debugf("Connected to %v", pd.Name())
	fcd.connectedLock.Lock()
	defer fcd.connectedLock.Unlock()

	fcd.connected = append(fcd.connected, connectTimeProxyDialer{
		ProxyDialer: pd,
		connectTime: connectTime,
	})
	sort.Sort(fcd.connected)

	// Set top dialer if the fastest dialer changed.
	td := fcd.topDialer.get()
	newTopDialer := fcd.connected[0].ProxyDialer
	if td != newTopDialer {
		log.Debugf("Setting new top dialer to %v", newTopDialer.Name())
		fcd.topDialer.set(newTopDialer)
	}
	fcd.onSuccess(fcd.topDialer.get())
	log.Debug("Finished adding connected dialer")
}

// parallelDial dials all the dialers in parallel to connect the user as quickly as
// possible on startup.
func (fcd *fastConnectDialer) connectAll(dialers []ProxyDialer) {
	if len(dialers) == 0 {
		log.Errorf("No dialers to connect to")
		return
	}
	log.Debugf("Dialing all dialers in parallel %#v", dialers)
	// Loop until we're connected
	for len(fcd.connected) < 2 {
		fcd.parallelDial(dialers)
		// Add jitter to avoid thundering herd
		time.Sleep(time.Duration(rand.Intn(4000)) * time.Millisecond)
	}

	// At this point, we've tried all of the dialers, and they've all either
	// succeeded or failed.

	// If we've connected to more than one dialer after trying all of them,
	// switch to the next dialer that's optimized for multiple connections.
	nextOpts := fcd.opts.Clone()
	nextOpts.Dialers = fcd.proxyDialers()
	fcd.next(nextOpts, fcd)
}

func (fcd *fastConnectDialer) parallelDial(dialers []ProxyDialer) {
	log.Debug("Connecting to all dialers")
	var wg sync.WaitGroup
	for index, d := range dialers {
		wg.Add(1)
		go func(pd ProxyDialer, index int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
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
		}(d, index)
	}
	wg.Wait()
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
