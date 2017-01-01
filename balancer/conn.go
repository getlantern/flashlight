package balancer

import (
	"github.com/getlantern/flashlight/ops"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ioTimeout       = "i/o timeout"
	ioTimeoutLength = 11
	nanosPerSecond  = 1000 * 1000
)

// conn wraps a net.Conn and tracks statistics on data transfer, throughput and
// success of connection.
type conn struct {
	net.Conn
	origin   string
	onFinish func(op *ops.Op)
	sent     *rate
	recv     *rate
	firstErr error
	closed   int32
	errMx    sync.RWMutex
}

func wrap(wrapped net.Conn, origin string, onFinish func(op *ops.Op)) net.Conn {
	c := &conn{
		Conn:     wrapped,
		origin:   origin,
		onFinish: onFinish,
		sent:     &rate{},
		recv:     &rate{},
	}
	go c.track()
	return c
}

func (c *conn) track() {
	for {
		c.sent.snapshot()
		c.recv.snapshot()
		if atomic.LoadInt32(&c.closed) == 1 {
			c.report()
			return
		}
		time.Sleep(1 * time.Second)
	}
}

func (c *conn) report() {
	op := ops.Begin("xfer").Origin(c.origin, "")

	c.sent.mx.Lock()
	op.SetMetric("client_bytes_sent", float64(c.sent.total)).
		SetMetric("client_bps_sent_min", c.sent.min).
		SetMetric("client_bps_sent_max", c.sent.max).
		SetMetric("client_bps_sent_avg", c.sent.average())
	c.sent.mx.Unlock()

	c.recv.mx.Lock()
	op.SetMetric("client_bytes_recv", float64(c.recv.total)).
		SetMetric("client_bps_recv_min", c.recv.min).
		SetMetric("client_bps_recv_max", c.recv.max).
		SetMetric("client_bps_recv_avg", c.recv.average())
	c.recv.mx.Unlock()

	if c.onFinish != nil {
		c.onFinish(op)
	}
	c.errMx.RLock()
	op.FailIf(c.firstErr)
	c.errMx.RUnlock()

	log.Debug("Finished xfer")
	op.End()
}

func (c *conn) Write(b []byte) (int, error) {
	c.sent.begin(time.Now)
	n, err := c.Conn.Write(b)
	c.sent.update(n, time.Now())
	if err != nil && !isTimeout(err) {
		c.storeError(err)
	}
	return n, err
}

func (c *conn) Read(b []byte) (int, error) {
	c.recv.begin(time.Now)
	n, err := c.Conn.Read(b)
	c.recv.update(n, time.Now())
	if err != nil && !isTimeout(err) && err != io.EOF {
		c.storeError(err)
	}
	return n, err
}

func (c *conn) Close() error {
	if atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return c.Conn.Close()
	}
	return nil
}

func (c *conn) storeError(err error) {
	c.errMx.Lock()
	if c.firstErr == nil {
		c.firstErr = err
	}
	c.errMx.Unlock()
}

func isTimeout(err error) bool {
	es := err.Error()
	esl := len(es)
	return esl >= ioTimeoutLength && es[esl-ioTimeoutLength:] == ioTimeout
}
