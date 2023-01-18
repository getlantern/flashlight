package shadowsocks

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	outlineShadowsocksNet "github.com/Jigsaw-Code/outline-ss-server/net"
	"github.com/Jigsaw-Code/outline-ss-server/service"
	"github.com/Jigsaw-Code/outline-ss-server/service/metrics"
)

// shadowsocks/local.go houses adapters for use with Lantern. This mostly is in
// place to allow the Lantern backend to handle upstream connections itself and
// have shadowsocks behave like other transports we use in Lantern.

var (
	ErrListenerClosed = errors.New("listener closed")

	defaultMetrics  metrics.ShadowsocksMetrics
	initMetricsOnce sync.Once
)

// This value is lifted from the the main server.go to match behavior
// 59 seconds is most common timeout for servers that do not respond to invalid requests
const tcpReadTimeout time.Duration = 59 * time.Second

// HandleLocalPredicate is a type of function that determines whether to handle an
// upstream address locally or not.  If the function returns true, the address is
// handled locally.  If the funtion returns false, the address is handled by the
// default upstream dial.
type HandleLocalPredicate func(addr string) bool

// AlwaysLocal is a HandleLocalPredicate that requests local handling for all addresses
func AlwaysLocal(addr string) bool { return true }

func maybeLocalDialer(isLocal HandleLocalPredicate, handleLocal service.TargetDialer, handleUpstream service.TargetDialer) service.TargetDialer {
	return func(tgtAddr string, clientAddr net.Addr, proxyMetrics *metrics.ProxyMetrics, targetIPValidator outlineShadowsocksNet.TargetIPValidator) (outlineShadowsocksNet.DuplexConn, *outlineShadowsocksNet.ConnectionError) {
		if isLocal(tgtAddr) {
			return handleLocal(tgtAddr, clientAddr, proxyMetrics, targetIPValidator)
		} else {
			return handleUpstream(tgtAddr, clientAddr, proxyMetrics, targetIPValidator)
		}
	}
}

type ListenerOptions struct {
	Listener              *net.TCPListener
	Ciphers               service.CipherList
	ReplayCache           *service.ReplayCache
	Metrics               metrics.ShadowsocksMetrics
	Timeout               time.Duration
	ShouldHandleLocally   HandleLocalPredicate                    // determines whether an upstream should be handled by the listener locally or dial upstream
	TargetIPValidator     outlineShadowsocksNet.TargetIPValidator // determines validity of non-local upstream dials
	MaxPendingConnections int                                     // defaults to 1000
}

// GetDefaultMetrics returns the singleton metrics.ShadowsocksMetrics instance
// (more than one cannot be created in a process without a panic)
func GetDefaultMetrics() metrics.ShadowsocksMetrics {
	initMetricsOnce.Do(func() {
		defaultMetrics = metrics.NewPrometheusShadowsocksMetrics(nil, prometheus.DefaultRegisterer)
	})
	return defaultMetrics
}

type llistener struct {
	service.TCPService
	wrapped      net.Listener
	connections  chan net.Conn
	closedSignal chan struct{}
	closeOnce    sync.Once
	closeError   error
}

// ListenLocalTCP creates a net.Listener that returns all inbound shadowsocks connections to the
// returned listener rather than dialing upstream. Any upstream or local handling should be handled by the
// caller of Accept().
func ListenLocalTCP(
	addr string, ciphers service.CipherList,
	replayHistory int,
) (net.Listener, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	l, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return nil, err
	}

	replayCache := service.NewReplayCache(replayHistory)

	options := &ListenerOptions{
		Listener:    l,
		Ciphers:     ciphers,
		ReplayCache: &replayCache,
	}

	return ListenLocalTCPOptions(options), nil
}

// ListenLocalTCPOptions creates a net.Listener that returns some inbound shadowsocks connections
// to the returned listener.  Which connnections are returned to the listener are determined
// by the ShouldHandleLocally predicate, which defaults to all connections.
// Any upstream handling should be handled by the caller of Accept() for any connection returned.
func ListenLocalTCPOptions(options *ListenerOptions) net.Listener {

	maxPending := options.MaxPendingConnections
	if maxPending == 0 {
		maxPending = DefaultMaxPending
	}

	l := &llistener{
		wrapped:      options.Listener,
		connections:  make(chan net.Conn, maxPending),
		closedSignal: make(chan struct{}),
	}

	timeout := options.Timeout
	if timeout == 0 {
		timeout = tcpReadTimeout
	}
	m := options.Metrics
	if m == nil {
		m = GetDefaultMetrics()
	}

	validator := options.TargetIPValidator
	if validator == nil {
		validator = outlineShadowsocksNet.RequirePublicIP
	}

	isLocal := options.ShouldHandleLocally
	if isLocal == nil {
		isLocal = AlwaysLocal
	}

	dialer := maybeLocalDialer(isLocal, l.dialPipe, service.DefaultDialTarget)
	l.TCPService = service.NewTCPService(
		options.Ciphers, options.ReplayCache,
		m, timeout,
		&service.TCPServiceOptions{
			DialTarget:        dialer,
			TargetIPValidator: validator,
		},
	)

	go func() {
		err := l.Serve(options.Listener)
		if err != nil {
			logger.Errorf("serving on %s: %v", l.Addr(), err)
		}
		l.Close()
	}()

	return l
}

// Accept implements Accept() from net.Listener
func (l *llistener) Accept() (net.Conn, error) {
	select {
	case conn, ok := <-l.connections:
		if !ok {
			return nil, ErrListenerClosed
		}
		return conn, nil
	case <-l.closedSignal:
		return nil, ErrListenerClosed
	}
}

// Close implements Close() from net.Listener
func (l *llistener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closedSignal)
		l.closeError = l.Stop()
	})
	return l.closeError
}

// Addr implements Addr() from net.Listener
func (l *llistener) Addr() net.Addr {
	return l.wrapped.Addr()
}

// dialPipe is the dialer used by the shadowsocks tcp service when handling the upstream locally.
// When the shadowsocks TcpService dials upstream, one end of a duplex Pipe is returned to it
// and the other end is issued to the consumer of the Listener.
func (l *llistener) dialPipe(addr string, clientAddr net.Addr, proxyMetrics *metrics.ProxyMetrics, targetIPValidator outlineShadowsocksNet.TargetIPValidator) (outlineShadowsocksNet.DuplexConn, *outlineShadowsocksNet.ConnectionError) {
	c1, c2 := net.Pipe()

	// this is returned to the shadowsocks handler as the upstream connection
	a := metrics.MeasureConn(&lduplex{c1}, &proxyMetrics.ProxyTarget, &proxyMetrics.TargetProxy)

	// this is returned via the Listener as a client connection
	b := &lfwd{c2, clientAddr, addr}

	l.connections <- b

	return a, nil
}

// this is an adapter that fulfils the expectation
// of the shadowsocks handler that it can independently
// close the read and write on it's upstream side.
type lduplex struct {
	net.Conn
}

// this is triggered when the remote end is finished.
// This triggers a close of both ends.
func (l *lduplex) CloseRead() error {
	return l.Conn.Close()
}

// this is triggered when a client finishes writing,
// it is handled as a no-op, we just ignore it since
// we don't depend on half closing the connection to
// signal anything.
func (l *lduplex) CloseWrite() error {
	return nil
}

// this is an adapter that forwards the remote address
// on the "real" client connection to the consumer of
// the listener.  The real requested upstream address
// is also available if needed.
type lfwd struct {
	net.Conn
	remoteAddr     net.Addr
	upstreamTarget string
}

func (l *lfwd) RemoteAddr() net.Addr {
	return l.remoteAddr
}

func (l *lfwd) UpstreamTarget() string {
	return l.upstreamTarget
}
