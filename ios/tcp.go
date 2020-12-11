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
	dialTimeout = 30 * time.Second
)

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
	downstreams  *lruConnList
	upstreams    map[io.Closer]io.Closer
	mx           sync.RWMutex
}

func (h *proxiedTCPHandler) Handle(downstream net.Conn, addr *net.TCPAddr) error {
	// before handling a new connection, make sure we're okay on memory
	h.client.reduceMemoryPressureIfNecessary()

	host, ok := h.grabber.ReverseLookup(addr.IP)
	if !ok {
		return log.Errorf("Invalid ip address %v, will not connect", addr.IP)
	}

	addrString := fmt.Sprintf("%v:%d", host, addr.Port)
	atomic.AddInt64(&h.dialingConns, 1)
	ctx, cancelContext := context.WithTimeout(context.Background(), dialTimeout)
	upstream, err := h.dialOut(ctx, balancer.NetworkConnect, addrString)
	atomic.AddInt64(&h.dialingConns, -1)
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

	// keeps the connection fresh in the LRU cache as long as we're reading or writing to it
	keepFresh := func(_ int) {
		h.downstreams.mark(downstream)
	}

	outErr, inErr := netx.BidiCopyWithTracking(downstream, upstream, bufOut, bufIn, keepFresh, keepFresh)

	isIdled := idletiming.IsIdled(upstream)
	logError := func(err error) {
		if err == nil {
			return
		} else if isIdled {
			log.Debug(err)
		} else {
			log.Error(err)
		}
	}

	logError(outErr)
	logError(inErr)
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
