package listeners

import (
	"net"
	"net/http"
	"time"

	"github.com/getlantern/http-proxy/listeners"
	"github.com/juju/ratelimit"
)

type RateLimiter struct {
	r              *ratelimit.Bucket
	w              *ratelimit.Bucket
	rate           int64
	preferredMinIO int64
}

func NewRateLimiter(rate int64) *RateLimiter {
	l := &RateLimiter{
		rate: rate,
	}
	if rate > 0 {
		l.r = ratelimit.NewBucketWithRate(float64(rate), rate)
		l.w = ratelimit.NewBucketWithRate(float64(rate), rate)

		// prefer to read or write at least this number of bytes
		// at once when possible. Use a progressively lower min
		// for lower rates.
		min := rate / 8
		if min < 1 {
			min = 1
		} else if min > 512 {
			min = 512
		}
		l.preferredMinIO = min
	}
	return l
}

// Acquire up to max read tokens. Will return as soon as
// between min and max reads are acquired. Returns number
// of tokens acquired and boolean indicating whether they
// were immediately available or a delay was necessary.
func (l *RateLimiter) waitRead(min, max int64) (int64, bool) {
	t, d := l.wait(l.r, min, max)
	if d > 0 {
		time.Sleep(d)
		return t, true
	}
	return t, false
}

// Acquire up to max write tokens. Will return as soon as
// between min and max writes are acquired. Returns number
// of tokens acquired and boolean indicating whether they
// were immediately available or a delay was necessary.
func (l *RateLimiter) waitWrite(min, max int64) (int64, bool) {
	t, d := l.wait(l.w, min, max)
	if d > 0 {
		time.Sleep(d)
		return t, true
	}
	return t, false

}

func (l *RateLimiter) wait(b *ratelimit.Bucket, min, max int64) (int64, time.Duration) {
	if b == nil {
		return max, 0
	}

	var taken int64
	var d time.Duration

	if min > 0 {
		d = b.Take(min)
		taken = min
	}

	if d == 0 && taken < max {
		taken += b.TakeAvailable(max - taken)
	}

	return taken, d
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
		limiter:            NewRateLimiter(0),
	}, err
}

// Bitrate Conn wrapper
type bitrateConn struct {
	listeners.WrapConnEmbeddable
	net.Conn
	limiter *RateLimiter
}

func (c *bitrateConn) Read(p []byte) (n int, err error) {
	if c.limiter.rate == 0 {
		return c.Conn.Read(p)
	}

	var delayed bool
	var nn int
	// read in chunks until delayed or read ends
	for lp := int64(len(p)); lp > 0 && err == nil; lp = int64(len(p)) {
		bs := c.limiter.preferredMinIO
		if lp < bs {
			bs = lp
		}
		nn, err = c.Conn.Read(p[:bs])

		if nn > 0 {
			_, delayed = c.limiter.waitRead(int64(nn), int64(nn))
			n += nn
			p = p[nn:]
		}

		// short read or had to wait for tokens.
		if int64(nn) < bs || delayed {
			break
		}
	}
	return
}

func (c *bitrateConn) Write(p []byte) (n int, err error) {
	if c.limiter.rate == 0 {
		return c.Conn.Write(p)
	}

	var i int
	for lp := int64(len(p)); lp > 0 && err == nil; lp = int64(len(p)) {
		min := c.limiter.preferredMinIO
		if lp < min {
			min = lp
		}
		s, _ := c.limiter.waitWrite(min, lp)
		i, err = c.Conn.Write(p[:s])
		p = p[i:]
		n += i
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
	// pro-user message always overrides the active flag
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
