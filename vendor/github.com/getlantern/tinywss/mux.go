package tinywss

import (
	"context"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/ops"
	"github.com/pkg/errors"
	"github.com/xtaci/smux"
)

var _ Client = &smuxClient{}

var (
	errTimeout = &timeoutError{}
)

type smuxContext struct {
	session *smux.Session
	conn    net.Conn // the underlying Conn
}

type smuxClient struct {
	closed      uint64
	muClose     sync.RWMutex
	wrapped     *client
	config      *smux.Config
	chStreamReq chan streamReq
}

type smuxConn struct {
	net.Conn
	next net.Conn // next Wrapped (underlying Conn)
}

func (c *smuxConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	return n, translateSmuxErr(err)
}

func (c *smuxConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	return n, translateSmuxErr(err)
}

func (c *smuxConn) Close() error {
	return translateSmuxErr(c.Conn.Close())
}

func (c *smuxConn) SetDeadline(t time.Time) error {
	return translateSmuxErr(c.Conn.SetDeadline(t))
}

func (c *smuxConn) SetReadDeadline(t time.Time) error {
	return translateSmuxErr(c.Conn.SetReadDeadline(t))
}

func (c *smuxConn) SetWriteDeadline(t time.Time) error {
	return translateSmuxErr(c.Conn.SetWriteDeadline(t))
}

func (c *smuxConn) Wrapped() net.Conn {
	return c.next
}

func wrapClientSmux(c *client, opts *ClientOpts) Client {
	cfg := smux.DefaultConfig()
	if opts.KeepAliveInterval != 0 {
		cfg.KeepAliveInterval = opts.KeepAliveInterval
	}
	if opts.KeepAliveTimeout != 0 {
		cfg.KeepAliveTimeout = opts.KeepAliveTimeout
	}
	if opts.MaxFrameSize != 0 {
		cfg.MaxFrameSize = opts.MaxFrameSize
	}
	if opts.MaxReceiveBuffer != 0 {
		cfg.MaxReceiveBuffer = opts.MaxReceiveBuffer
	}

	var maxDials int64 = defaultMaxPendingDials
	if opts.MaxPendingDials > 0 {
		maxDials = opts.MaxPendingDials
	}
	sc := &smuxClient{
		wrapped:     c,
		config:      cfg,
		chStreamReq: make(chan streamReq, maxDials),
	}
	go sc.dialLoop()
	return sc
}

type streamResult struct {
	conn *smuxConn
	err  error
}

type streamReq struct {
	ch  chan streamResult
	ctx context.Context
}

func (c *smuxClient) dialLoop() {
	chSessionReq := make(chan struct{})
	defer close(chSessionReq)
	chSession := make(chan *smuxContext)
	var curSession *smuxContext

	go func() {
		for range chSessionReq {
			ses, err := c.newSession()
			if err != nil {
				continue
			}
			chSession <- ses
		}
		close(chSession)
	}()
	chSessionReq <- struct{}{}

	for req := range c.chStreamReq {
		if c.isClosed() {
			req.ch <- streamResult{nil, ErrClientClosed}
			continue
		}
		if curSession == nil {
			select {
			case chSessionReq <- struct{}{}:
			default:
			}
			select {
			case curSession = <-chSession:
			case <-req.ctx.Done():
				req.ch <- streamResult{nil, req.ctx.Err()}
				continue
			}
		}
		stream, err := curSession.session.OpenStream()
		if err != nil {
			curSession = nil
			req.ch <- streamResult{nil, err}
		} else {
			req.ch <- streamResult{&smuxConn{stream, curSession.conn}, nil}
		}
	}
	if curSession != nil {
		curSession.session.Close()
	}
}

func (c *smuxClient) newSession() (*smuxContext, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialSessionTimeout)
	defer cancel()
	conn, err := c.wrapped.DialContext(ctx)
	if err != nil {
		return nil, err
	}
	session, err := smux.Client(conn, c.config)
	err = translateSmuxErr(err)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &smuxContext{session, conn}, nil
}

// implements Client.DialContext
func (c *smuxClient) DialContext(ctx context.Context) (net.Conn, error) {
	// prevent writing to c.chStreamReq after channel closed
	c.muClose.RLock()
	if c.isClosed() {
		c.muClose.RUnlock()
		return nil, ErrClientClosed
	}
	ch := make(chan streamResult)
	select {
	case c.chStreamReq <- streamReq{ch, ctx}:
	default:
		return nil, errors.New("maximum pending dials reached")
	}
	c.muClose.RUnlock()
	res := <-ch
	return res.conn, res.err
}

// implements Client.Close
func (c *smuxClient) Close() error {
	c.muClose.Lock()
	atomic.StoreUint64(&c.closed, 1)
	close(c.chStreamReq)
	c.muClose.Unlock()
	return nil
}

func (c *smuxClient) isClosed() bool {
	return atomic.LoadUint64(&c.closed) == 1
}

func translateSmuxErr(err error) error {
	err = errors.Cause(err)
	if err == nil {
		return err
	} else if _, ok := err.(net.Error); ok {
		return err
	} else if strings.Contains(err.Error(), "timeout") { // certain newer versions
		return errTimeout
	} else {
		return err
	}
}

var _ net.Error = &timeoutError{}

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

var _ net.Listener = &smuxListener{}

type smuxListener struct {
	wrapped               *listener
	connections           chan net.Conn
	closed                chan struct{}
	config                *smux.Config
	numConnections        int64
	numVirtualConnections int64
}

func wrapListenerSmux(l *listener, opts *ListenOpts) (net.Listener, error) {
	cfg := smux.DefaultConfig()
	if opts.KeepAliveInterval != 0 {
		cfg.KeepAliveInterval = opts.KeepAliveInterval
	}
	if opts.KeepAliveTimeout != 0 {
		cfg.KeepAliveTimeout = opts.KeepAliveTimeout
	}
	if opts.MaxFrameSize != 0 {
		cfg.MaxFrameSize = opts.MaxFrameSize
	}
	if opts.MaxReceiveBuffer != 0 {
		cfg.MaxReceiveBuffer = opts.MaxReceiveBuffer
	}

	ll := &smuxListener{
		wrapped:     l,
		connections: make(chan net.Conn, 1000),
		closed:      make(chan struct{}),
		config:      cfg,
	}

	ops.Go(ll.listen)
	ops.Go(ll.logStats)
	return ll, nil
}

func (l *smuxListener) listen() {
	defer l.Close()
	for {
		conn, err := l.wrapped.Accept()
		if err != nil {
			if err != ErrListenerClosed {
				log.Errorf("tinywss mux listener: %s", err)
			}
			return
		}
		l.handleConn(conn)
	}
}

func (l *smuxListener) handleConn(conn net.Conn) {
	wconn, ok := conn.(*WsConn)
	if !ok {
		log.Errorf("not handling unexpected connection type")
		conn.Close()
		return
	}
	atomic.AddInt64(&l.numConnections, 1)

	if strings.EqualFold(wconn.protocol, ProtocolMux) {
		ops.Go(func() {
			l.handleSession(wconn)
			atomic.AddInt64(&l.numConnections, -1)
		})
	} else {
		// not multiplexed
		wconn.onClose = func() {
			atomic.AddInt64(&l.numConnections, -1)
		}
		l.connections <- conn
	}
}

func (l *smuxListener) handleSession(conn *WsConn) {
	session, err := smux.Server(conn, l.config)
	err = translateSmuxErr(err)
	if err != nil {
		log.Errorf("error handing mux connection: %s", err)
	}

	defer session.Close()

	for {
		stream, err := session.AcceptStream()
		err = translateSmuxErr(err)
		if err != nil {
			log.Debugf("accepting stream: %v", err)
			return
		}
		atomic.AddInt64(&l.numVirtualConnections, 1)
		l.connections <- &WsConn{
			Conn:     &smuxConn{stream, conn},
			protocol: ProtocolMux,
			onClose: func() {
				atomic.AddInt64(&l.numVirtualConnections, -1)
			},
			headers: cloneHeaders(conn.UpgradeHeaders()),
		}
	}
}

// implements net.Listener.Accept
func (l *smuxListener) Accept() (net.Conn, error) {
	select {
	case conn, ok := <-l.connections:
		if !ok {
			return nil, ErrListenerClosed
		}
		return conn, nil
	case <-l.closed:
		return nil, ErrListenerClosed
	}
}

// implements net.Listener.Close
func (l *smuxListener) Close() error {
	select {
	case <-l.closed:
		return nil
	default:
		close(l.closed)
		return l.wrapped.Close()
	}
}

func (l *smuxListener) Addr() net.Addr {
	return l.wrapped.Addr()
}

func (l *smuxListener) logStats() {
	for {
		select {
		case <-time.After(5 * time.Second):
			log.Debugf("Connections: %d   Virtual: %d", atomic.LoadInt64(&l.numConnections), atomic.LoadInt64(&l.numVirtualConnections))
		case <-l.closed:
			log.Debugf("Connections: %d   Virtual: %d", atomic.LoadInt64(&l.numConnections), atomic.LoadInt64(&l.numVirtualConnections))
			log.Debug("Done logging stats.")
			return
		}
	}
}
