package dialer

import (
	"context"
	"math/rand"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/flashlight/v7/stats"
)

// ConnectivityCheckDialer finds a working dialer as quickly as possible.
type ConnectivityCheckDialer struct {
	dialers           []ProxyDialer
	onError           func(error, bool)
	onSuccess         func(ProxyDialer)
	statsTracker      stats.Tracker
	connectTimeDialer *connectTimeDialer

	activeDialer atomic.Value

	next func(*Options) (Dialer, error)
	opts *Options
}

// DialContext implements Dialer.
func (ccd *ConnectivityCheckDialer) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	return ccd.activeDialer.Load().(Dialer).DialContext(ctx, network, addr)
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
	connectedChan chan bool
	connected     dialersByConnectTime
	// Lock for the slice of dialers.
	sync.RWMutex
}

func (ctd *connectTimeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// Use the dialer with the lowest connect time, waiting on early dials for any
	// connections at all.
	td := ctd.loadTopDialer()
	conn, up, err := td.DialContext(ctx, network, addr)
	//conn, up, err := td.DialContext(ctx, network, addr)
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
func (d *connectTimeDialer) proxyDialers() []ProxyDialer {
	d.RLock()
	defer d.RUnlock()
	dialers := make([]ProxyDialer, len(d.connected))
	for i, ctd := range d.connected {
		dialers[i] = ctd.ProxyDialer
	}
	return dialers
}

func (ctd *connectTimeDialer) onConnected(pd ProxyDialer, connectTime time.Duration) {
	log.Debugf("Connected to %v", pd.Name())
	ctd.Lock()
	defer ctd.Unlock()
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
		ctd.connectedChan <- true
	}
}

func (ctd *connectTimeDialer) loadTopDialer() ProxyDialer {
	ctd.RLock()
	defer ctd.RUnlock()
	return ctd.topDialer
}

func (ctd *connectTimeDialer) storeTopDialer(pd ProxyDialer) {
	ctd.Lock()
	defer ctd.Unlock()
	ctd.topDialer = pd
}

// NewConnectivityCheckDialer creates a new dialer for checking proxy connectivity.
func NewConnectivityCheckDialer(opts *Options, next func(opts *Options) (Dialer, error)) (Dialer, error) {
	if opts.OnError == nil {
		opts.OnError = func(error, bool) {}
	}
	if opts.OnSuccess == nil {
		opts.OnSuccess = func(ProxyDialer) {}
	}
	if opts.StatsTracker == nil {
		opts.StatsTracker = stats.NewNoop()
	}

	log.Debugf("Creating startup with %d dialers", len(opts.Dialers))

	ctd := &connectTimeDialer{
		connected:     make(dialersByConnectTime, 0),
		connectedChan: make(chan bool),
	}
	ctd.storeTopDialer(newWaitForConnectionDialer(ctd.connectedChan))

	sd := &ConnectivityCheckDialer{
		dialers:           opts.Dialers,
		onError:           opts.OnError,
		onSuccess:         opts.OnSuccess,
		statsTracker:      opts.StatsTracker,
		connectTimeDialer: ctd,
		next:              next,
		opts:              opts,
	}
	sd.activeDialer.Store(ctd)

	sd.parallelDial()

	return sd, nil
}

// parallelDial dials all the dialers in parallel to connect the user as quickly as
// possible on startup.
func (ccd *ConnectivityCheckDialer) parallelDial() {
	if len(ccd.dialers) == 0 {
		log.Errorf("No dialers to connect to")
		return
	}
	log.Debugf("Dialing all dialers in parallel %#v", ccd.dialers)
	// Loop until we're connected
	for len(ccd.connectTimeDialer.connected) < 2 {
		ccd.connectAll()
		// Add jitter to avoid thundering herd
		time.Sleep(time.Duration(rand.Intn(4000)) * time.Millisecond)
	}

	// If we've connected to more than one dialer after trying all of them,
	// switch to the next dialer that's optimized for multiple connections.
	nextOpts := ccd.opts.Clone()
	nextOpts.Dialers = ccd.connectTimeDialer.proxyDialers()
	nextDialer, err := ccd.next(nextOpts)
	if err != nil {
		log.Errorf("Could not create next dialer? ", err)
	} else {
		ccd.activeDialer.Store(nextDialer)
	}
}

func (ccd *ConnectivityCheckDialer) connectAll() {
	var wg sync.WaitGroup
	for index, d := range ccd.dialers {
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
			ccd.statsTracker.SetHasSucceedingProxy(true)
			ccd.connectTimeDialer.onConnected(pd, time.Since(start))
		}(d, index)
	}
	wg.Wait()
}
