package obfs4listener

import (
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/withtimeout"

	pt "git.torproject.org/pluggable-transports/goptlib.git"
	"gitlab.com/yawning/obfs4.git/transports/base"
	"gitlab.com/yawning/obfs4.git/transports/obfs4"
)

const (
	DefaultHandshakeConcurrency          = 1024
	DefaultMaxPendingHandshakesPerClient = 512
	DefaultHandshakeTimeout              = 10 * time.Second
)

var (
	log = golog.LoggerFor("obfs4listener")
)

func Wrap(wrapped net.Listener, stateDir string, handshakeConcurrency int, maxPendingHandshakesPerClient int, handshakeTimeout time.Duration) (net.Listener, error) {
	err := os.MkdirAll(stateDir, 0700)
	if err != nil {
		return nil, fmt.Errorf("Unable to make statedir at %v: %v", stateDir, err)
	}

	tr := &obfs4.Transport{}
	sf, err := tr.ServerFactory(stateDir, &pt.Args{})
	if err != nil {
		return nil, fmt.Errorf("Unable to create obfs4 server factory: %v", err)
	}

	if handshakeConcurrency <= 0 {
		handshakeConcurrency = DefaultHandshakeConcurrency
	}
	if maxPendingHandshakesPerClient <= 0 {
		maxPendingHandshakesPerClient = DefaultMaxPendingHandshakesPerClient
	}
	if handshakeTimeout <= 0 {
		handshakeTimeout = DefaultHandshakeTimeout
	}

	log.Debugf("Handshake Concurrency: %d", handshakeConcurrency)
	log.Debugf("Max Pending Handshakes Per Client: %d", maxPendingHandshakesPerClient)
	log.Debugf("Handshake Timeout: %v", handshakeTimeout)

	var clientsFinished sync.WaitGroup

	ol := &obfs4listener{
		handshakeTimeout:              handshakeTimeout,
		maxPendingHandshakesPerClient: maxPendingHandshakesPerClient,
		wrapped:                       wrapped,
		sf:                            sf,
		clientsFinished:               &clientsFinished,
		clients:                       make(map[string]*client),
		pending:                       make(chan net.Conn, handshakeConcurrency),
		ready:                         make(chan *result),
	}

	go ol.accept()
	for i := 0; i < handshakeConcurrency; i++ {
		go ol.wrapPending()
	}
	go ol.monitor()
	return ol, nil
}

type client struct {
	newConns chan net.Conn
	wg       *sync.WaitGroup
}

type result struct {
	conn net.Conn
	err  error
}

type obfs4listener struct {
	handshakeTimeout              time.Duration
	maxPendingHandshakesPerClient int
	wrapped                       net.Listener
	sf                            base.ServerFactory
	clientsFinished               *sync.WaitGroup
	clients                       map[string]*client
	pending                       chan net.Conn
	ready                         chan *result
	numClients                    int64
	handshaking                   int64
	closeMx                       sync.Mutex
	closed                        bool
}

func (l *obfs4listener) Accept() (net.Conn, error) {
	r, ok := <-l.ready
	if !ok {
		return nil, fmt.Errorf("Closed")
	}
	return r.conn, r.err
}

func (l *obfs4listener) Addr() net.Addr {
	return l.wrapped.Addr()
}

func (l *obfs4listener) Close() error {
	l.closeMx.Lock()
	defer l.closeMx.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true

	err := l.wrapped.Close()
	go func() {
		// Drain ready
		for result := range l.ready {
			if result.conn != nil {
				result.conn.Close()
			}
		}
	}()
	return err
}

func (l *obfs4listener) accept() {
	defer func() {
		for _, client := range l.clients {
			close(client.newConns)
		}
		l.clientsFinished.Wait()
		close(l.pending)
		close(l.ready)
	}()

	for {
		conn, err := l.wrapped.Accept()
		if err != nil {
			l.ready <- &result{nil, err}
			return
		}
		// WrapConn does a handshake with the client, which involves io operations
		// and can time out. We do it on a separate goroutine, but we limit it to
		// one goroutine per remote address.
		remoteAddr := conn.RemoteAddr().String()
		remoteHost, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			log.Errorf("Unable to determine host for address %v: %v", remoteAddr, err)
			conn.Close()
			continue
		}
		cl := l.clients[remoteHost]
		if cl == nil {
			cl = &client{
				newConns: make(chan net.Conn, l.maxPendingHandshakesPerClient),
				wg:       l.clientsFinished,
			}
			l.clientsFinished.Add(1)
			l.clients[remoteHost] = cl
			atomic.AddInt64(&l.numClients, 1)
			go cl.wrapIncoming(l.pending)
		}
		select {
		case cl.newConns <- conn:
			// will handshake
		default:
			log.Errorf("Too many pending handshakes for client at %v, ignoring new connections", remoteAddr)
			conn.Close()
		}
	}
}

func (c *client) wrapIncoming(pending chan net.Conn) {
	for conn := range c.newConns {
		pending <- conn
	}
}

func (l *obfs4listener) wrapPending() {
	for conn := range l.pending {
		l.wrap(conn)
	}
}

func (l *obfs4listener) wrap(conn net.Conn) {
	atomic.AddInt64(&l.handshaking, 1)
	defer atomic.AddInt64(&l.handshaking, -1)
	start := time.Now()
	_wrapped, timedOut, err := withtimeout.Do(l.handshakeTimeout, func() (interface{}, error) {
		o, err := l.sf.WrapConn(conn)
		if err != nil {
			return nil, err
		}
		return &obfs4Conn{Conn: o, wrapped: conn}, nil
	})

	if timedOut {
		log.Tracef("Handshake with %v timed out", conn.RemoteAddr())
		conn.Close()
	} else if err != nil {
		log.Tracef("Handshake error with %v: %v", conn.RemoteAddr(), err)
		conn.Close()
	} else {
		log.Tracef("Successful obfs4 handshake in %v", time.Now().Sub(start))
		l.ready <- &result{_wrapped.(net.Conn), err}
	}
}

func (l *obfs4listener) monitor() {
	for {
		time.Sleep(5 * time.Second)
		log.Debugf("Number of clients: %d", atomic.LoadInt64(&l.numClients))
		log.Debugf("Connections waiting to start handshaking: %d", len(l.pending))
		log.Debugf("Currently handshaking connections: %d", atomic.LoadInt64(&l.handshaking))
	}
}

type obfs4Conn struct {
	net.Conn
	wrapped net.Conn
}

func (conn *obfs4Conn) Wrapped() net.Conn {
	return conn.wrapped
}
