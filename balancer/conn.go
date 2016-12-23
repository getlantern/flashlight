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
	origin    string
	onFinish  func(op *ops.Op)
	sent      int64
	sendStart int64
	sendEnd   int64
	recvStart int64
	recvEnd   int64
	recv      int64
	firstErr  error
	closed    int32
	errMx     sync.RWMutex
}

func wrap(wrapped net.Conn, origin string, onFinish func(op *ops.Op)) net.Conn {
	return &conn{
		Conn:     wrapped,
		origin:   origin,
		onFinish: onFinish,
	}
}

func (c *conn) Write(b []byte) (int, error) {
	atomic.CompareAndSwapInt64(&c.sendStart, 0, time.Now().UnixNano())
	n, err := c.Conn.Write(b)
	if n > 0 {
		atomic.StoreInt64(&c.sendEnd, time.Now().UnixNano())
		atomic.AddInt64(&c.sent, int64(n))
	}
	if err != nil && !isTimeout(err) {
		c.storeError(err)
	}
	return n, err
}

func (c *conn) Read(b []byte) (int, error) {
	atomic.CompareAndSwapInt64(&c.recvStart, 0, time.Now().UnixNano())
	n, err := c.Conn.Read(b)
	if n > 0 {
		atomic.StoreInt64(&c.recvEnd, time.Now().UnixNano())
		atomic.AddInt64(&c.recv, int64(n))
	}
	if err != nil && !isTimeout(err) && err != io.EOF {
		c.storeError(err)
	}
	return n, err
}

func (c *conn) Close() error {
	if atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		err := c.Conn.Close()
		sent := float64(atomic.LoadInt64(&c.sent))
		recv := float64(atomic.LoadInt64(&c.recv))
		sendNanos := float64(atomic.LoadInt64(&c.sendEnd) - atomic.LoadInt64(&c.sendStart))
		recvNanos := float64(atomic.LoadInt64(&c.recvEnd) - atomic.LoadInt64(&c.recvStart))
		sentPerSecond := float64(0)
		if sendNanos > 0 {
			sentPerSecond = sent * nanosPerSecond / sendNanos
		}
		recvPerSecond := float64(0)
		if recvNanos > 0 {
			recvPerSecond = recv * nanosPerSecond / recvNanos
		}
		op := ops.Begin("xfer").
			Set("client_bytes_sent", sent).
			Set("client_conn_bytes_sent_per_second", sentPerSecond).
			Set("client_bytes_recv", recv).
			Set("client_conn_bytes_recv_per_second", recvPerSecond).
			Origin(c.origin, "")
		if c.onFinish != nil {
			c.onFinish(op)
		}
		c.errMx.RLock()
		op.FailIf(c.firstErr)
		c.errMx.RUnlock()
		op.End()
		return err
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
