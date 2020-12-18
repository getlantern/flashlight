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

type dialRequest struct {
	ctx      context.Context
	addr     string
	upstream chan net.Conn
	err      chan error
}

// proxiedTCPHandler implements TCPConnHandler from go-tun2socks by routing TCP connections
// via our proxies.
type proxiedTCPHandler struct {
	dialOut               func(ctx context.Context, network, addr string) (net.Conn, error)
	client                *client
	grabber               dnsgrab.Server
	mtu                   int
	dialWorker            *worker
	downstreamWriteWorker *worker
	upstreams             map[io.Closer]io.Closer
	dialingConns          int64
	copyingConns          int64
	mx                    sync.RWMutex
}

func newProxiedTCPHandler(c *client, bal *balancer.Balancer, grabber dnsgrab.Server) *proxiedTCPHandler {
	result := &proxiedTCPHandler{
		dialOut:               bal.DialContext,
		client:                c,
		grabber:               grabber,
		mtu:                   c.mtu,
		dialWorker:            newWorker(0),
		downstreamWriteWorker: newWorker(10),
		upstreams:             make(map[io.Closer]io.Closer),
	}
	go result.trackStats()
	return result
}

func (h *proxiedTCPHandler) Handle(downstream net.Conn, addr *net.TCPAddr) error {
	host, ok := h.grabber.ReverseLookup(addr.IP)
	if !ok {
		return log.Errorf("Invalid ip address %v, will not connect", addr.IP)
	}

	ctx, cancelContext := context.WithTimeout(context.Background(), dialTimeout)
	addrString := fmt.Sprintf("%v:%d", host, addr.Port)

	// MEMORY_OPTIMIZATION dialing is very memory intensive because of the cryptography involved, so we limit the
	// concurrency of dialing to keep our memory usage under control
	req := &dialRequest{
		ctx:      ctx,
		addr:     addrString,
		upstream: make(chan net.Conn),
		err:      make(chan error),
	}
	var upstream net.Conn
	var err error
	doDial := func() {
		upstream, err := h.dialOut(req.ctx, balancer.NetworkConnect, req.addr)
		if err != nil {
			req.err <- err
			return
		}
		req.upstream <- upstream
	}

	atomic.AddInt64(&h.dialingConns, 1)
	h.dialWorker.tasks <- doDial
	select {
	case upstream = <-req.upstream:
		// okay
	case err = <-req.err:
		// error
	}
	atomic.AddInt64(&h.dialingConns, -1)

	if err != nil {
		cancelContext()
		return log.Errorf("Unable to dial %v: %v", addrString, err)
	}

	h.mx.Lock()
	h.upstreams[downstream] = upstream
	h.mx.Unlock()

	go func() {
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
	defer func() {
		h.mx.Lock()
		delete(h.upstreams, downstream)
		h.mx.Unlock()
	}()

	// Note - we don't pool these as pooling seems to create additional memory pressure somehow
	bufOut := make([]byte, h.mtu)
	bufIn := make([]byte, h.mtu)

	closeTimer := time.NewTimer(closeTimeout)
	keepFresh := func(_ int) {
		if !closeTimer.Stop() {
			<-closeTimer.C
		}
		closeTimer.Reset(closeTimeout)
	}

	// MEMORY_OPTIMIZATION use a threadLimitingTCPConn to limit the number of goroutines that write to lwip
	outErrCh, inErrCh := netx.BidiCopyWithOpts(upstream, newThreadLimitingTCPConn(downstream, h.downstreamWriteWorker), &netx.CopyOpts{
		BufOut: bufOut,
		BufIn:  bufIn,
		OnOut:  keepFresh,
		OnIn:   keepFresh,
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

func (h *proxiedTCPHandler) trackStats() {
	for {
		h.mx.RLock()
		numUpstreams := len(h.upstreams)
		h.mx.RUnlock()
		statsLog.Debugf("TCP Conns    Upstreams: %d   Dialing: %d   Copying: %d", numUpstreams, atomic.LoadInt64(&h.dialingConns), atomic.LoadInt64(&h.copyingConns))
		time.Sleep(1 * time.Second)
	}
}
