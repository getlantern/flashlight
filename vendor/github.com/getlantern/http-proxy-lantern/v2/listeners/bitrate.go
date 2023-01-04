package listeners

import (
	"net"
	"net/http"
	"time"

	"github.com/getlantern/http-proxy/listeners"
	"github.com/getlantern/ratelimit"
)

const (
	minSleep = 5 * time.Millisecond // don't bother sleeping for less than this amount of time
)

type RateLimiter struct {
	r         *ratelimit.Bucket
	w         *ratelimit.Bucket
	rateRead  int64
	rateWrite int64
}

func NewRateLimiter(rateRead, rateWrite int64) *RateLimiter {
	l := &RateLimiter{
		rateRead:  rateRead,
		rateWrite: rateWrite,
	}
	if rateRead > 0 {
		l.r = ratelimit.NewBucketWithRate(float64(rateRead), rateRead)
	}
	if rateWrite > 0 {
		l.w = ratelimit.NewBucketWithRate(float64(rateWrite), rateWrite)
	}
	return l
}

func (l *RateLimiter) GetRateRead() int64 {
	return l.rateRead
}

func (l *RateLimiter) GetRateWrite() int64 {
	return l.rateWrite
}

func (l *RateLimiter) waitRead(n int) {
	d := l.wait(l.r, n)
	if d > 0 {
		sleep(d)
	}
}

func (l *RateLimiter) waitWrite(n int) {
	d := l.wait(l.w, n)
	if d > 0 {
		sleep(d)
	}
}

// In order to avoid lots of very short (and relatively expensive) sleeps, never sleep for
// less than minSleep.
func sleep(d time.Duration) {
	if d < minSleep {
		d = minSleep
	}
	time.Sleep(d)
}

func (l *RateLimiter) wait(b *ratelimit.Bucket, n int) time.Duration {
	return b.Take(int64(n))
}

type bitrateListener struct {
	net.Listener
}

func NewBitrateListener(l net.Listener) net.Listener {
	return &bitrateListener{l}
}

func (bl *bitrateListener) Accept() (net.Conn, error) {
	c, err := bl.Listener.Accept()
	if err != nil {
		return nil, err
	}

	wc, _ := c.(listeners.WrapConnEmbeddable)
	return &bitrateConn{
		WrapConnEmbeddable: wc,
		Conn:               c,
		limiter:            NewRateLimiter(0, 0),
	}, err
}

// Bitrate Conn wrapper
type bitrateConn struct {
	listeners.WrapConnEmbeddable
	net.Conn
	limiter *RateLimiter
}

func (c *bitrateConn) Read(p []byte) (n int, err error) {
	if c.limiter.rateRead == 0 {
		return c.Conn.Read(p)
	}

	n, err = c.Conn.Read(p)
	if err == nil {
		c.limiter.waitRead(n)
	}
	return
}

func (c *bitrateConn) Write(p []byte) (n int, err error) {
	if c.limiter.rateWrite == 0 {
		return c.Conn.Write(p)
	}

	n, err = c.Conn.Write(p)
	if err == nil {
		c.limiter.waitWrite(n)
	}
	return
}

func (c *bitrateConn) OnState(s http.ConnState) {
	// Pass down to wrapped connections
	if c.WrapConnEmbeddable != nil {
		c.WrapConnEmbeddable.OnState(s)
	}
}

func (c *bitrateConn) ControlMessage(msgType string, data interface{}) {
	// per user message always overrides the active flag
	if msgType == "throttle" {
		c.limiter = data.(*RateLimiter)
	}

	if c.WrapConnEmbeddable != nil {
		c.WrapConnEmbeddable.ControlMessage(msgType, data)
	}
}

func (c *bitrateConn) Wrapped() net.Conn {
	return c.Conn
}
