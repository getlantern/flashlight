// Package cmux provides multiplexing over net.Conns using smux and adhering
// to standard net package interfaces.
package cmux

import (
	"net"
	"sync"
	"time"

	"github.com/getlantern/golog"
	"github.com/xtaci/smux"
)

var (
	log               = golog.LoggerFor("cmux")
	defaultBufferSize = 4194304
	errTimeout        = &timeoutError{}
)

type cmconn struct {
	net.Conn
	onClose func()
	closed  bool
	mx      sync.Mutex
}

func (c *cmconn) Close() error {
	c.mx.Lock()
	defer c.mx.Unlock()
	if c.closed {
		return nil
	}
	err := c.Conn.Close()
	c.onClose()
	c.closed = true
	return translateSmuxErr(err)
}

func (c *cmconn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	return n, translateSmuxErr(err)
}

func (c *cmconn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	return n, translateSmuxErr(err)
}

func (c *cmconn) SetDeadline(t time.Time) error {
	return translateSmuxErr(c.Conn.SetDeadline(t))
}

func (c *cmconn) SetReadDeadline(t time.Time) error {
	return translateSmuxErr(c.Conn.SetReadDeadline(t))
}

func (c *cmconn) SetWriteDeadline(t time.Time) error {
	return translateSmuxErr(c.Conn.SetWriteDeadline(t))
}

func translateSmuxErr(err error) error {
	if err == smux.ErrTimeout {
		return errTimeout
	} else {
		return err
	}
}

var _ net.Error = &timeoutError{}

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
