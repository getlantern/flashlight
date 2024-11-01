package dialer

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/getlantern/flashlight/v7/stats"
)

// FastConnectDialer finds a working dialer as quickly as possible.
type FastConnectDialer struct {
	dialers           []ProxyDialer
	onError           func(error, bool)
	onSuccess         func(ProxyDialer)
	statsTracker      stats.Tracker
	connectTimeDialer *connectTimeDialer

	activeDialer     Dialer
	activeDialerLock sync.RWMutex

	next func(*Options) (Dialer, error)
	opts *Options
}

// DialContext implements Dialer.
func (ccd *FastConnectDialer) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	td := ccd.loadActiveDialer()
	if td == nil {
		return nil, errors.New("no active dialer")
	}
	return td.DialContext(ctx, network, addr)
}

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

type connectTimeDialer struct {
	topDialer     ProxyDialer
	topDialerLock sync.RWMutex
	connected     dialersByConnectTime
	connectedChan chan int
	// Lock for the slice of dialers.
	connectedLock sync.RWMutex
}

func (ctd *connectTimeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// Use the dialer with the lowest connect time, waiting on early dials for any
	// connections at all.
	td := ctd.loadTopDialer()
	if td == nil {
		log.Debug("No top dialer")
		return nil, errors.New("no top dialer")
	}
	conn, up, err := td.DialContext(ctx, network, addr)
	if err != nil {
		// Error connecting to the proxy or to the destination
		if up {
			// Error connecting to the destination
			log.Debugf("Error connecting to upstream destination %v: %v", addr, err)
		} else {
			// Error connecting to the proxy
			log.Debugf("Error connecting to proxy %v: %v", td.Name(), err)

		}
		return nil, err
	}
	return conn, err

}

// Accessor for a copy of the ProxyDialer slice
func (cdt *connectTimeDialer) proxyDialers() []ProxyDialer {
	cdt.connectedLock.RLock()
	defer cdt.connectedLock.RUnlock()
	dialers := make([]ProxyDialer, len(cdt.connected))
	for i, ctd := range cdt.connected {
		dialers[i] = ctd.ProxyDialer
	}
	return dialers
}

func (ctd *connectTimeDialer) onConnected(pd ProxyDialer, connectTime time.Duration) {
	log.Debugf("Connected to %v", pd.Name())
	ctd.connectedLock.Lock()
	defer ctd.connectedLock.Unlock()

	ctd.connected = append(ctd.connected, connectTimeProxyDialer{
		ProxyDialer: pd,
		connectTime: connectTime,
	})
	sort.Sort(ctd.connected)

	// Set top dialer if the fastest dialer changed.
	td := ctd.loadTopDialer()
	newTopDialer := ctd.connected[0].ProxyDialer
	if td != newTopDialer {
		ctd.storeTopDialer(newTopDialer)
	}
	log.Debug("Finished adding connected dialer")
}

func (ctd *connectTimeDialer) loadTopDialer() ProxyDialer {
	ctd.topDialerLock.RLock()
	defer ctd.topDialerLock.RUnlock()
	return ctd.topDialer
}

func (ctd *connectTimeDialer) storeTopDialer(pd ProxyDialer) {
	ctd.topDialerLock.Lock()
	defer ctd.topDialerLock.Unlock()
	ctd.topDialer = pd
}

// NewFastConnectDialer creates a new dialer for checking proxy connectivity.
func NewFastConnectDialer(opts *Options, next func(opts *Options) (Dialer, error)) (Dialer, error) {
	if opts.OnError == nil {
		opts.OnError = func(error, bool) {}
	}
	if opts.OnSuccess == nil {
		opts.OnSuccess = func(ProxyDialer) {}
	}
	if opts.StatsTracker == nil {
		opts.StatsTracker = stats.NewNoop()
	}

	log.Debugf("Creating new dialer with %d dialers", len(opts.Dialers))

	ctd := &connectTimeDialer{
		connected:     make(dialersByConnectTime, 0),
		connectedChan: make(chan int),
	}
	//ctd.storeTopDialer(newWaitForConnectionDialer(ctd.connectedChan))

	fcd := &FastConnectDialer{
		dialers:           opts.Dialers,
		onError:           opts.OnError,
		onSuccess:         opts.OnSuccess,
		statsTracker:      opts.StatsTracker,
		connectTimeDialer: ctd,
		next:              next,
		opts:              opts,
	}
	fcd.storeActiveDialer(ctd)

	fcd.parallelDial()

	return fcd, nil
}

// parallelDial dials all the dialers in parallel to connect the user as quickly as
// possible on startup.
func (fcd *FastConnectDialer) parallelDial() {
	if len(fcd.dialers) == 0 {
		log.Errorf("No dialers to connect to")
		return
	}
	log.Debugf("Dialing all dialers in parallel %#v", fcd.dialers)
	// Loop until we're connected
	for len(fcd.connectTimeDialer.connected) < 2 {
		fcd.connectAll()
		// Add jitter to avoid thundering herd
		time.Sleep(time.Duration(rand.Intn(4000)) * time.Millisecond)
	}

	// If we've connected to more than one dialer after trying all of them,
	// switch to the next dialer that's optimized for multiple connections.
	nextOpts := fcd.opts.Clone()
	nextOpts.Dialers = fcd.connectTimeDialer.proxyDialers()
	nextDialer, err := fcd.next(nextOpts)
	if err != nil {
		log.Errorf("Could not create next dialer? ", err)
	} else {
		log.Debug("Switching to next dialer")
		fcd.storeActiveDialer(nextDialer)
	}
}

func (fcd *FastConnectDialer) connectAll() {
	log.Debug("Connecting to all dialers")
	var wg sync.WaitGroup
	for index, d := range fcd.dialers {
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
			fcd.statsTracker.SetHasSucceedingProxy(true)
			fcd.connectTimeDialer.onConnected(pd, time.Since(start))
		}(d, index)
	}
	wg.Wait()
}

func (fcd *FastConnectDialer) storeActiveDialer(active Dialer) {
	fcd.activeDialerLock.Lock()
	defer fcd.activeDialerLock.Unlock()
	fcd.activeDialer = active
}

func (fcd *FastConnectDialer) loadActiveDialer() Dialer {
	fcd.activeDialerLock.RLock()
	defer fcd.activeDialerLock.RUnlock()
	return fcd.activeDialer
}
