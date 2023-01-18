// Package reconn provides an implementation of net.Conn that allows
// replaying a limited amount of the data that was read from the underlying
// conn. This can be used to implement look aheads on top of other net.Conns.
package reconn

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
)

var ErrOverflowed = errors.New("replay buffer overflowed")

// Conn is a net.Conn that supports replaying.
type Conn struct {
	net.Conn
	limit      int
	buf        *bytes.Buffer
	overflowed bool
	mx         sync.Mutex
}

// Wrap wraps the supplied conn, allowing replay from the current point up to
// the given limit.
func Wrap(conn net.Conn, limit int) *Conn {
	return &Conn{
		Conn:  conn,
		limit: limit,
		buf:   bytes.NewBuffer(make([]byte, 0, limit)),
	}
}

// Read implements the method from net.Conn with added support for replaying.
func (conn *Conn) Read(b []byte) (n int, err error) {
	// Read rest from underlying connection
	var _n int
	_n, err = conn.Conn.Read(b)
	n += _n
	conn.mx.Lock()
	if !conn.overflowed && n > 0 {
		neededForBuf := conn.limit - conn.buf.Len()
		toCopy := neededForBuf
		if n > neededForBuf {
			conn.overflowed = true
		} else if n < neededForBuf {
			toCopy = n
		}
		if toCopy > 0 {
			// Fill buffer
			conn.buf.Write(b[:toCopy])
		}
	}
	conn.mx.Unlock()
	return
}

// Rereader returns an io.Reader that reads from the last marked point. If the
// connection has overflowed, this returns an error.
func (conn *Conn) Rereader() (io.Reader, error) {
	if conn.overflowed {
		return nil, ErrOverflowed
	}
	return &rereader{conn}, nil
}

type rereader struct {
	conn *Conn
}

func (rr *rereader) Read(b []byte) (n int, err error) {
	rr.conn.mx.Lock()
	n, err = rr.conn.buf.Read(b)
	rr.conn.mx.Unlock()
	return
}
