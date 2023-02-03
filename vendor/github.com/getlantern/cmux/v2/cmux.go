// Package cmux provides multiplexing over net.Conns using smux and adhering
// to standard net package interfaces.
package cmux

import (
	"net"
	"sync"
	"time"

	"github.com/getlantern/golog"
)

var (
	log             = golog.LoggerFor("cmux")
	ErrTimeout      = &timeoutError{}
	defaultProtocol = NewSmuxProtocol(nil)
)

type ErrorMapperFn func(error) error

type cmconn struct {
	net.Conn
	onClose        func()
	closed         bool
	mx             sync.Mutex
	translateError ErrorMapperFn
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
	return c.translateError(err)
}

func (c *cmconn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	return n, c.translateError(err)
}

func (c *cmconn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	return n, c.translateError(err)
}

func (c *cmconn) SetDeadline(t time.Time) error {
	return c.translateError(c.Conn.SetDeadline(t))
}

func (c *cmconn) SetReadDeadline(t time.Time) error {
	return c.translateError(c.Conn.SetReadDeadline(t))
}

func (c *cmconn) SetWriteDeadline(t time.Time) error {
	return c.translateError(c.Conn.SetWriteDeadline(t))
}

var _ net.Error = &timeoutError{}

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

type Session interface {
	OpenStream() (net.Conn, error)
	AcceptStream() (net.Conn, error)
	Close() error
	NumStreams() int
}

type Protocol interface {
	Client(net.Conn) (Session, error)
	Server(net.Conn) (Session, error)
	TranslateError(error) error
}
