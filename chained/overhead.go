package chained

import (
	"net"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"

	log "github.com/sirupsen/logrus"
)

var (
	wireTraffic uint64
	appTraffic  uint64
)

func init() {
	go func() {
		for {
			time.Sleep(30 * time.Second)
			wire := atomic.LoadUint64(&wireTraffic)
			app := atomic.LoadUint64(&appTraffic)
			log.Debugf("App Traffic: %v  Wire Traffic: %v  Overhead: %3.2f%%", humanize.Bytes(app), humanize.Bytes(wire), float64(wire-app)*100/float64(app))
		}
	}()
}

func overheadDialer(app bool, dial func(network, addr string, timeout time.Duration) (net.Conn, error)) func(network, addr string, timeout time.Duration) (net.Conn, error) {
	return func(network, addr string, timeout time.Duration) (net.Conn, error) {
		return dialOverhead(network, addr, timeout, app, dial)
	}
}

func dialOverhead(network, addr string, timeout time.Duration, app bool, dial func(network, addr string, timeout time.Duration) (net.Conn, error)) (net.Conn, error) {
	return overheadWrapper(app)(dial(network, addr, timeout))
}

func overheadWrapper(app bool) func(net.Conn, error) (net.Conn, error) {
	return func(conn net.Conn, err error) (net.Conn, error) {
		if conn == nil || err != nil {
			return conn, err
		}
		oc := &overheadconn{Conn: conn}
		if app {
			oc.traffic = &appTraffic
		} else {
			oc.traffic = &wireTraffic
		}
		return oc, err
	}
}

type overheadconn struct {
	net.Conn
	traffic *uint64
}

func (c *overheadconn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if err == nil {
		atomic.AddUint64(c.traffic, uint64(n))
	}
	return n, err
}

func (c *overheadconn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if err == nil {
		atomic.AddUint64(c.traffic, uint64(n))
	}
	return n, err
}
