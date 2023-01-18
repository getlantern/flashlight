package quicwrapper

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/ops"
	quic "github.com/lucas-clemente/quic-go"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	connectionCounts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "quic_connections",
			Help: "Connections that the quic wrapper is currently tracking",
		},
		[]string{"type"},
	)
)

// ListenAddr creates a QUIC server listening on a given address.
// The net.Conn instances returned by the net.Listener may be multiplexed connections.
func ListenAddr(addr string, tlsConf *tls.Config, config *Config) (net.Listener, error) {
	tlsConf = defaultNextProtos(tlsConf, DefaultServerProtos)
	ql, err := quic.ListenAddr(addr, tlsConf, config)
	if err != nil {
		return nil, err
	}
	return listen(ql, tlsConf, config)
}

// Listen creates a QUIC server listening on a given net.PacketConn
// The net.Conn instances returned by the net.Listener may be multiplexed connections.
// The caller is responsible for closing the net.PacketConn after the listener has been
// closed.
func Listen(pconn net.PacketConn, tlsConf *tls.Config, config *Config) (net.Listener, error) {
	tlsConf = defaultNextProtos(tlsConf, DefaultServerProtos)
	ql, err := quic.Listen(pconn, tlsConf, config)
	if err != nil {
		return nil, err
	}
	return listen(ql, tlsConf, config)
}

func listen(ql quic.Listener, tlsConf *tls.Config, config *Config) (net.Listener, error) {
	l := &listener{
		quicListener: ql,
		config:       config,
		connections:  make(chan net.Conn, 1000),
		acceptError:  make(chan error, 1),
		closedSignal: make(chan struct{}),
	}
	ops.Go(l.listen)
	ops.Go(l.logStats)

	return l, nil
}

var _ net.Listener = &listener{}

// wraps quic.Listener to create a net.Listener
type listener struct {
	numConnections        int64
	numVirtualConnections int64
	quicListener          quic.Listener
	config                *Config
	connections           chan net.Conn
	acceptError           chan error
	closedSignal          chan struct{}
	closeErr              error
	closeOnce             sync.Once
}

// implements net.Listener.Accept
func (l *listener) Accept() (net.Conn, error) {
	select {
	case conn, ok := <-l.connections:
		if !ok {
			return nil, ErrListenerClosed
		}
		return conn, nil
	case err, ok := <-l.acceptError:
		if !ok {
			return nil, ErrListenerClosed
		}
		return nil, err
	case <-l.closedSignal:
		return nil, ErrListenerClosed
	}
}

// implements net.Listener.Close
// Shut down the QUIC listener.
// this implicitly sends CONNECTION_CLOSE frames to peers
// note: it is still the responsibility of the caller
// to call Close() on any Conn returned from Accept()
func (l *listener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closedSignal)
		l.closeErr = l.quicListener.Close()
	})
	return l.closeErr
}

func (l *listener) isClosed() bool {
	select {
	case <-l.closedSignal:
		return true
	default:
		return false
	}
}

// implements net.Listener.Addr
func (l *listener) Addr() net.Addr {
	return l.quicListener.Addr()
}

func (l *listener) listen() {
	group := &sync.WaitGroup{}

	defer func() {
		l.Close()
		close(l.acceptError)
		// wait for writers to exit, drain connections
		group.Wait()
		close(l.connections)
		for c := range l.connections {
			c.Close()
		}

		log.Debugf("Listener finished with Connections: %d Virtual: %d", atomic.LoadInt64(&l.numConnections), atomic.LoadInt64(&l.numVirtualConnections))
	}()

	for {
		session, err := l.quicListener.Accept(context.Background())
		if err != nil {
			if !l.isClosed() {
				l.acceptError <- err
			}
			return
		}
		if l.isClosed() {
			session.CloseWithError(0, "")
			return
		} else {
			atomic.AddInt64(&l.numConnections, 1)
			group.Add(1)
			ops.Go(func() {
				l.handleSession(session)
				atomic.AddInt64(&l.numConnections, -1)
				group.Done()
			})
		}
	}
}

func (l *listener) handleSession(session quic.Connection) {

	// keep a smoothed average of the bandwidth estimate
	// for the session
	bw := NewEMABandwidthSampler(session)
	bw.Start()

	// track active session connections
	active := make(map[quic.StreamID]Conn)
	var mx sync.Mutex

	defer func() {
		bw.Stop()
		session.CloseWithError(0, "")

		// snapshot any non-closed connections, then nil out active list
		// conns being closed will 'remove' themselves from the nil
		// list, not the snapshot.
		var snapshot map[quic.StreamID]Conn
		mx.Lock()
		snapshot = active
		active = nil
		mx.Unlock()

		// immediately close any connections that are still active
		for _, conn := range snapshot {
			conn.Close()
		}
	}()

	for {
		stream, err := session.AcceptStream(context.Background())
		if err != nil {
			if isPeerGoingAway(err) {
				log.Tracef("Accepting stream: Peer going away (%v)", err)
				return
			} else {
				log.Errorf("Accepting stream: %v", err)
				return
			}
		} else {
			atomic.AddInt64(&l.numVirtualConnections, 1)
			conn := newConn(stream, session, bw, func(id quic.StreamID) {
				atomic.AddInt64(&l.numVirtualConnections, -1)
				// remove conn from active list
				mx.Lock()
				delete(active, id)
				mx.Unlock()
			})

			l.connections <- conn
		}
	}
}

func (l *listener) logStats() {
	for {
		select {
		case <-time.After(5 * time.Second):
			if !l.isClosed() {
				log.Debugf("Connections: %d   Virtual: %d", atomic.LoadInt64(&l.numConnections), atomic.LoadInt64(&l.numVirtualConnections))
				connectionCounts.WithLabelValues("connections").Set(float64(atomic.LoadInt64(&l.numConnections)))
				connectionCounts.WithLabelValues("virtual").Set(float64(atomic.LoadInt64(&l.numVirtualConnections)))
			}
		case <-l.closedSignal:
			log.Debugf("Connections: %d   Virtual: %d", atomic.LoadInt64(&l.numConnections), atomic.LoadInt64(&l.numVirtualConnections))
			log.Debug("Done logging stats.")
			connectionCounts.WithLabelValues("connections").Set(0)
			connectionCounts.WithLabelValues("virtual").Set(0)
			return
		}
	}
}
