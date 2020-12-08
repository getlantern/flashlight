package ios

import (
	"context"
	"fmt"
	"net"
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
	grabber      dnsgrab.Server
	mtu          int
	dialingConns int64
	copyingConns int64
	mruConns     *mruConnList
}

func (h *proxiedTCPHandler) Handle(downstream net.Conn, addr *net.TCPAddr) error {
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

	go func() {
		h.mruConns.mark(downstream)
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
	defer h.mruConns.remove(downstream)

	// Note - we don't pool these as pooling seems to create additional memory pressure somehow
	bufOut := make([]byte, h.mtu)
	bufIn := make([]byte, h.mtu)

	// keeps the connection fresh in the LRU cache as long as we're reading or writing to it
	keepFresh := func(_ int) {
		h.mruConns.mark(downstream)
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

func (h *proxiedTCPHandler) disconnect() {
	conns := h.mruConns.removeAll()
	for conn := range conns {
		conn.Close()
	}
}

func (h *proxiedTCPHandler) trackStats() {
	for {
		statsLog.Debugf("TCP Conns    Active: %d    Dialing: %d   Copying: %d", h.mruConns.len(), atomic.LoadInt64(&h.dialingConns), atomic.LoadInt64(&h.copyingConns))
		time.Sleep(1 * time.Second)
	}
}
