package cmux

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtaci/smux"
)

var (
	ErrClosed = errors.New("listener closed")
)

type ListenOpts struct {
	Listener          net.Listener
	BufferSize        int
	KeepAliveInterval time.Duration
}

type listener struct {
	ListenOpts
	nextConn              chan net.Conn
	nextErr               chan error
	sessions              map[int]*smux.Session
	nextSessionID         int
	closeOnce             sync.Once
	chClosed              chan struct{}
	numConnections        int64
	numVirtualConnections int64
	mx                    sync.Mutex
}

// Listen creates a net.Listener that multiplexes connections over a connection
// obtained from the underlying opts.Listener.
func Listen(opts *ListenOpts) net.Listener {
	if opts.BufferSize <= 0 {
		opts.BufferSize = defaultBufferSize
	}
	l := &listener{
		ListenOpts: *opts,
		nextConn:   make(chan net.Conn, 1000),
		nextErr:    make(chan error, 1),
		sessions:   make(map[int]*smux.Session),
		chClosed:   make(chan struct{}),
	}
	go l.listen()
	go l.logStats()
	return l
}

func (l *listener) listen() {
	defer l.Close()
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			l.nextErr <- err
			return
		}
		atomic.AddInt64(&l.numConnections, 1)
		go l.handleConn(conn)
	}
}

func (l *listener) handleConn(conn net.Conn) {
	smuxConfig := smux.DefaultConfig()
	smuxConfig.MaxReceiveBuffer = l.BufferSize
	if l.KeepAliveInterval > 0 {
		smuxConfig.KeepAliveInterval = l.KeepAliveInterval
	}
	session, err := smux.Server(conn, smuxConfig)
	if err != nil {
		l.nextErr <- err
		return
	}
	l.mx.Lock()
	sessionID := l.nextSessionID
	l.nextSessionID++
	l.sessions[sessionID] = session
	l.mx.Unlock()

	defer func() {
		session.Close()
		conn.Close()
		l.mx.Lock()
		delete(l.sessions, sessionID)
		l.mx.Unlock()
		atomic.AddInt64(&l.numConnections, -1)
	}()

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			log.Debugf("Error creating multiplexed session, probably just means that the underlying connection was closed: %v", err)
			return
		}
		atomic.AddInt64(&l.numVirtualConnections, 1)
		l.nextConn <- &cmconn{
			Conn:    stream,
			onClose: l.cmconnClosed,
		}
	}
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case <-l.chClosed:
		return nil, ErrClosed
	case conn := <-l.nextConn:
		return conn, nil
	case err := <-l.nextErr:
		return nil, err
	}
}

func (l *listener) Close() (err error) {
	l.closeOnce.Do(func() {
		close(l.chClosed)
		l.mx.Lock()
		defer l.mx.Unlock()
		for _, session := range l.sessions {
			closeErr := session.Close()
			if closeErr != nil {
				log.Errorf("Error closing session: %v", closeErr)
			}
		}
		err = l.Listener.Close()
		// Drain nextConn and nextErr
		for {
			select {
			case conn := <-l.nextConn:
				conn.Close()
			case <-l.nextErr:
			default:
				return
			}
		}
	})
	return
}

func (l *listener) Addr() net.Addr {
	return l.Listener.Addr()
}

func (l *listener) cmconnClosed() {
	atomic.AddInt64(&l.numVirtualConnections, -1)
}

func (l *listener) logStats() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			log.Debugf("Connections: %d   Virtual: %d", atomic.LoadInt64(&l.numConnections), atomic.LoadInt64(&l.numVirtualConnections))
		case <-l.chClosed:
			log.Debug("Done logging stats")
			return
		}
	}
}
