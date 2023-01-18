package bufconn

import (
	"bufio"
	"net"
)

// Conn is a net.Conn that also exposes a bufio.Reader on top of that conn.
// The bufio.Reader is intended to be used first, after which it is safe to read
// from the Conn. Even if the buffered reader buffered more than necessary to
// serve its consumer, no data will be lost when starting to read from the Conn.
// Once reading from the Conn has begun, Head() should not be called and may
// return a nil bufio.Reader or one in an inconsistent state.
type Conn interface {
	net.Conn

	// Head exposes a bufio.Reader on top of this Conn meant to read the beginning
	// portion (head) of the data.
	Head() *bufio.Reader
}

type conn struct {
	net.Conn
	br *bufio.Reader
}

// Wrap wraps a net.Conn with support for save buffered reading of the head.
func Wrap(wrapped net.Conn) Conn {
	return &conn{
		wrapped,
		bufio.NewReader(wrapped),
	}
}

func (c *conn) Read(b []byte) (n int, err error) {
	if c.br != nil {
		n, err = c.br.Read(b)
		if c.br.Buffered() == 0 {
			c.br = nil
		}
		return n, err
	}
	return c.Conn.Read(b)
}

func (c *conn) Head() *bufio.Reader {
	return c.br
}

func (c *conn) Wrapped() net.Conn {
	return c.Conn
}
