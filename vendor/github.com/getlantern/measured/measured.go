package measured

import (
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/mtime"
)

// Stats provides statistics about total transfer and rates, all in bytes.
type Stats struct {
	SentTotal int
	SentMin   float64
	SentMax   float64
	SentAvg   float64
	RecvTotal int
	RecvMin   float64
	RecvMax   float64
	RecvAvg   float64
	// Duration indicates how long it has been since the connection was opened
	// (more precisely, how long it's been since it was wrapped by measured).
	Duration time.Duration
}

// Conn is a wrapped net.Conn that exposes statistics about transfer data and
// the first error encountered during processing.
type Conn interface {
	net.Conn

	// Stats gets the stats over the lifetime of the connection
	Stats() *Stats

	// FirstError gets the the first unexpected error encountered during network
	// processing. If this is not nil, something went wrong.
	FirstError() error

	// Wrapped() exposes the wrapped net.Conn
	Wrapped() net.Conn
}

// conn wraps a net.Conn and tracks statistics on data transfer, throughput
// and success of connection.
type conn struct {
	net.Conn
	startTime        time.Time
	onFinish         func(Conn)
	sent             rater
	recv             rater
	firstErr         error
	closed           int32
	trackingFinished chan bool
	statsMx          sync.RWMutex
	errMx            sync.RWMutex
}

// Wrap wraps a connection into a measured Conn that recalculates rates at the
// given interval.
func Wrap(wrapped net.Conn, rateInterval time.Duration, onFinish func(Conn)) Conn {
	c := &conn{
		Conn:             wrapped,
		startTime:        time.Now(),
		onFinish:         onFinish,
		trackingFinished: make(chan bool),
	}
	go c.track(rateInterval)
	return c
}

func (c *conn) Stats() *Stats {
	stats := &Stats{}
	stats.SentTotal, stats.SentMin, stats.SentMax, stats.SentAvg = c.sent.get()
	stats.RecvTotal, stats.RecvMin, stats.RecvMax, stats.RecvAvg = c.recv.get()
	stats.Duration = time.Now().Sub(c.startTime)
	return stats
}

func (c *conn) FirstError() error {
	c.errMx.RLock()
	firstErr := c.firstErr
	c.errMx.RUnlock()
	return firstErr
}

func (c *conn) Wrapped() net.Conn {
	return c.Conn
}

func (c *conn) track(rateInterval time.Duration) {
	for {
		c.sent.calc()
		c.recv.calc()
		if atomic.LoadInt32(&c.closed) == 1 {
			c.trackingFinished <- true
			if c.onFinish != nil {
				c.onFinish(c)
			}
			return
		}
		time.Sleep(rateInterval)
	}
}

func (c *conn) Write(b []byte) (int, error) {
	c.sent.begin(mtime.Now)
	n, err := c.Conn.Write(b)
	c.sent.advance(n, mtime.Now())
	if err != nil && !isTimeout(err) {
		c.storeError(err)
	}
	return n, err
}

func (c *conn) Read(b []byte) (int, error) {
	c.recv.begin(mtime.Now)
	n, err := c.Conn.Read(b)
	c.recv.advance(n, mtime.Now())
	if err != nil && !isTimeout(err) && err != io.EOF {
		c.storeError(err)
	}
	return n, err
}

func (c *conn) Close() error {
	if atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		err := c.Conn.Close()
		// Wait for tracking to finish
		<-c.trackingFinished
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
	var nerr net.Error
	if ok := errors.As(err, &nerr); ok && nerr.Timeout() {
		return true
	}
	return false
}
