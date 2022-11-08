package rpc

import (
	"net"
	"sync"
	"time"

	"github.com/golang/snappy"
)

func snappyDialer(d func(string, time.Duration) (net.Conn, error)) func(addr string, timeout time.Duration) (net.Conn, error) {
	return func(addr string, timeout time.Duration) (net.Conn, error) {
		return snappyWrap(d(addr, timeout))
	}
}

type SnappyListener struct {
	net.Listener
}

func (sl *SnappyListener) Accept() (net.Conn, error) {
	return snappyWrap(sl.Listener.Accept())
}

func snappyWrap(conn net.Conn, err error) (net.Conn, error) {
	if err != nil {
		return nil, err
	}
	r := snappy.NewReader(conn)
	// Note we don't use a buffered writer here as it doesn't seem to work well
	// with gRPC for some reason. In particular, without explicitly flushing, it
	// sometimes doesn't transmit messages completely, and with explicit flushing
	// the throughput seems low. TODO: figure out if we can buffer here.
	w := snappy.NewWriter(conn)
	sc := &snappyConn{Conn: conn, r: r, w: w}
	return sc, nil
}

type snappyConn struct {
	net.Conn
	r  *snappy.Reader
	w  *snappy.Writer
	mx sync.Mutex
}

func (sc *snappyConn) Read(p []byte) (int, error) {
	return sc.r.Read(p)
}

func (sc *snappyConn) Write(p []byte) (int, error) {
	sc.mx.Lock()
	n, err := sc.w.Write(p)
	sc.mx.Unlock()
	return n, err
}

func (sc *snappyConn) Close() error {
	sc.mx.Lock()
	sc.w.Close()
	sc.mx.Unlock()
	return sc.Conn.Close()
}
