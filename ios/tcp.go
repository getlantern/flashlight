package ios

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/dnsgrab"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/netx"
)

const (
	dialTimeout        = 30 * time.Second
	closeTimeout       = 1 * time.Second
	maxConcurrentDials = 1
)

type dialRequest struct {
	ctx      context.Context
	addr     string
	upstream chan net.Conn
	err      chan error
}

// proxiedTCPHandler implements TCPConnHandler from go-tun2socks by routing TCP connections
// via our proxies.
type proxiedTCPHandler struct {
	dialOut      func(ctx context.Context, network, addr string) (net.Conn, error)
	client       *client
	grabber      dnsgrab.Server
	mtu          int
	dialingConns int64
	copyingConns int64
	closingConns int64
	dialRequests chan *dialRequest
	downstreams  *lruConnList
	upstreams    map[io.Closer]io.Closer
	mx           sync.RWMutex
}

func newProxiedTCPHandler(c *client, bal *balancer.Balancer, grabber dnsgrab.Server) *proxiedTCPHandler {
	result := &proxiedTCPHandler{
		dialOut:      bal.DialContext,
		client:       c,
		grabber:      grabber,
		mtu:          c.mtu,
		dialRequests: make(chan *dialRequest),
		downstreams:  newLRUConnList(),
		upstreams:    make(map[io.Closer]io.Closer),
	}
	go result.handleDials()
	go result.trackStats()
	return result
}

// dialing is very memory intensive because of the cryptography involved, so we limit the concurrency of dialing to keep our memory usage
// under control
func (h *proxiedTCPHandler) handleDials() {
	for i := 0; i < maxConcurrentDials; i++ {
		go h.handleDial()
	}
}

func (h *proxiedTCPHandler) handleDial() {
	for req := range h.dialRequests {
		upstream, err := h.dialOut(req.ctx, balancer.NetworkConnect, req.addr)
		if err == nil {
			req.upstream <- upstream
		} else {
			req.err <- err
		}
	}
}

func (h *proxiedTCPHandler) Handle(downstream net.Conn, addr *net.TCPAddr) error {
	// before handling a new connection, make sure we're okay on memory
	h.client.reduceMemoryPressureIfNecessary()

	host, ok := h.grabber.ReverseLookup(addr.IP)
	if !ok {
		return log.Errorf("Invalid ip address %v, will not connect", addr.IP)
	}

	ctx, cancelContext := context.WithTimeout(context.Background(), dialTimeout)
	addrString := fmt.Sprintf("%v:%d", host, addr.Port)
	req := &dialRequest{
		ctx:      ctx,
		addr:     addrString,
		upstream: make(chan net.Conn),
		err:      make(chan error),
	}
	var upstream net.Conn
	var err error

	atomic.AddInt64(&h.dialingConns, 1)
	h.dialRequests <- req
	atomic.AddInt64(&h.dialingConns, -1)

	select {
	case upstream = <-req.upstream:
		// okay
	case err = <-req.err:
		// error
	}

	if err != nil {
		cancelContext()
		return log.Errorf("Unable to dial %v: %v", addrString, err)
	}

	h.mx.Lock()
	h.upstreams[downstream] = upstream
	h.mx.Unlock()

	go func() {
		h.downstreams.mark(downstream)
		h.copy(downstream, upstream)
		cancelContext()
	}()

	return nil
}

func (h *proxiedTCPHandler) copy(downstream net.Conn, upstream net.Conn) {
	atomic.AddInt64(&h.copyingConns, 1)
	defer atomic.AddInt64(&h.copyingConns, -1)

	defer downstream.Close()
	defer upstream.Close()
	defer h.downstreams.remove(downstream)
	defer func() {
		h.mx.Lock()
		delete(h.upstreams, downstream)
		h.mx.Unlock()
	}()

	// Note - we don't pool these as pooling seems to create additional memory pressure somehow
	bufOut := make([]byte, h.mtu)
	bufIn := make([]byte, h.mtu)

	closeTimer := time.NewTimer(closeTimeout)
	keepFresh := func() {
		h.downstreams.mark(downstream)
		if !closeTimer.Stop() {
			<-closeTimer.C
		}
		closeTimer.Reset(closeTimeout)
	}

	outErrCh, inErrCh := netx.BidiCopyReturningChannels(upstream, downstream, bufOut, bufIn, func(n int) {
		keepFresh()
	}, func(n int) {
		keepFresh()
		// // stall if memory gets low
		// h.client.memcap.get(n)
	})

	logError := func(err error) {
		if err == nil {
			return
		} else if idletiming.IsIdled(upstream) {
			log.Debug(err)
		} else {
			log.Error(err)
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()

		outErr := <-outErrCh
		logError(outErr)
		select {
		case inErr := <-inErrCh:
			logError(inErr)
		case <-closeTimer.C:
			log.Trace("opposite direction idle for more than closeTimeout, close everything")
			upstream.Close()
			downstream.Close()
		}
	}()

	go func() {
		defer wg.Done()

		inErr := <-inErrCh
		logError(inErr)
		select {
		case outErr := <-outErrCh:
			logError(outErr)
		case <-closeTimer.C:
			log.Trace("opposite direction idle for more than closeTimeout, close everything")
			upstream.Close()
			downstream.Close()
		}
	}()

	wg.Wait()
}

func (h *proxiedTCPHandler) closeOldestConn() bool {
	downstream, ok := h.downstreams.removeOldest()
	if ok {
		h.forceClose(downstream)
	}
	return ok
}

func (h *proxiedTCPHandler) disconnect() {
	downstreams := h.downstreams.removeAll()
	for downstream := range downstreams {
		// Close can actually take a while due to logic in both idletiming and measured, so to speed up our forced close, close downstream and upstream at the same time
		// and on goroutines
		go h.forceClose(downstream)
	}
}

func (h *proxiedTCPHandler) forceClose(downstream io.Closer) {
	h.mx.RLock()
	upstream := h.upstreams[downstream.(net.Conn)]
	h.mx.RUnlock()

	h.closeConn(downstream)
	if upstream != nil {
		h.closeConn(upstream)
	}
}

func (h *proxiedTCPHandler) closeConn(conn io.Closer) {
	atomic.AddInt64(&h.closingConns, 1)
	conn.Close()
	atomic.AddInt64(&h.closingConns, -1)
}

func (h *proxiedTCPHandler) trackStats() {
	for {
		h.mx.RLock()
		numUpstreams := len(h.upstreams)
		h.mx.RUnlock()
		statsLog.Debugf("TCP Conns    Downstreams: %d    Upstreams: %d   Dialing: %d   Copying: %d   Closing: %d", h.downstreams.len(), numUpstreams, atomic.LoadInt64(&h.dialingConns), atomic.LoadInt64(&h.copyingConns), atomic.LoadInt64(&h.closingConns))
		time.Sleep(1 * time.Second)
	}
}
