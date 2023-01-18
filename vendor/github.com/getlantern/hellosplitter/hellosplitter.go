// Package hellosplitter is used to split TLS ClientHello messages across multiple TCP packets. To
// achieve this, use hellosplitter's Conn as a transport in a TLS client connection.
package hellosplitter

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/getlantern/golog"
	"github.com/getlantern/tlsutil"
)

var log = golog.LoggerFor("hellosplitter")

// A BufferedWriteError occurs when a Conn attempts to write buffered data and fails.
type BufferedWriteError struct {
	BufferedData []byte

	// Written is the number of bytes from BufferedData which were successfully written to the
	// underlying transport.
	Written int

	cause error
}

func (err BufferedWriteError) Error() string {
	msg := "failed to write all buffered data to transport"
	if err.cause != nil {
		return fmt.Sprintf("%s: %v", msg, err.cause)
	}
	return msg
}

// Unwrap supports Go 1.13-style error unwrapping.
func (err BufferedWriteError) Unwrap() error {
	return err.cause
}

// A HelloParsingError occurs when a Conn fails to parse buffered data as a ClientHello.
type HelloParsingError struct {
	BufferedData []byte
	cause        error
}

func (err HelloParsingError) Error() string {
	return fmt.Sprintf("failed to parse buffered data as client hello: %v", err.cause)
}

// Unwrap supports Go 1.13-style error unwrapping.
func (err HelloParsingError) Unwrap() error {
	return err.cause
}

// A SplitFunc defines how a ClientHello is split.
type SplitFunc func([]byte) [][]byte

// Conn is intended for use as a transport in a TLS client connection. When Conn is used in this
// manner, the ClientHello will be split across multiple TCP packets. The ClientHello should be the
// first record written to this connection.
type Conn struct {
	net.Conn

	splitFunc SplitFunc

	// Everything passed to Write gets written to helloBuf until (a) we have processed a full
	// ClientHello or (b) we are sure that what we have processed could not constitute be part of a
	// ClientHello.
	helloBuf *bytes.Buffer

	wroteHello     bool
	wroteHelloLock sync.Mutex

	// True iff SetNoDelay(false) was called before the ClientHello was sent. Also protected by
	// wroteHelloLock.
	queuedNoDelayFalse bool
}

// Wrap the input connection with a hello-splitting connection. The ClientHello should be the first
// record written to the returned connection.
//
// If conn is a *net.TCPConn, then TCP_NODELAY will be configured on the connection. This is a
// requirement for splitting the ClientHello. To override this behavior for transmissions after the
// ClientHello, use Conn.SetNoDelay.
//
// If conn is not a *net.TCPConn, it must mimic TCP_NODELAY (sending packets as soon as they are
// ready). If the host OS does not support TCP_NODELAY, hello-splitting may not function as desired.
func Wrap(conn net.Conn, f SplitFunc) *Conn {
	return &Conn{conn, f, new(bytes.Buffer), false, sync.Mutex{}, false}
}

func (c *Conn) Write(b []byte) (n int, err error) {
	n, err = c.checkHello(b)
	if err != nil || n == len(b) {
		return
	}
	var m int
	m, err = c.Conn.Write(b[n:])
	return n + m, err
}

// SetNoDelay behaves like net.TCPConn.SetNoDelay except that the choice only applies after the
// ClientHello. Until the ClientHello is set, TCP_NODELAY will always be used.
//
// This is a no-op if the underlying transport is not a *net.TCPConn.
func (c *Conn) SetNoDelay(noDelay bool) error {
	c.wroteHelloLock.Lock()
	defer c.wroteHelloLock.Unlock()
	if c.wroteHello {
		if tcpConn, ok := c.Conn.(*net.TCPConn); ok {
			return tcpConn.SetNoDelay(noDelay)
		}
	} else if !noDelay {
		c.queuedNoDelayFalse = true
	}
	return nil
}

// Returns the number of bytes written *from b*. Note that more bytes may be written to the
// underlying transport if part of the hello was previously buffered.
func (c *Conn) checkHello(b []byte) (int, error) {
	c.wroteHelloLock.Lock()
	defer c.wroteHelloLock.Unlock()
	if !c.wroteHello {
		c.helloBuf.Write(b) // Writes to bytes.Buffers do not return errors.
		nHello, parseErr := tlsutil.ValidateClientHello(c.helloBuf.Bytes())
		tcpConn, isTCPConn := c.Conn.(*net.TCPConn)
		if isTCPConn {
			// No delay is the default, but we set it for good measure. If the type check fails and
			// the transport is not a net.TCPConn, we just assume the user knows what they're doing.
			if err := tcpConn.SetNoDelay(true); err != nil {
				log.Errorf("failed to set TCP_NODELAY: %v", err)
			}
		}
		if parseErr == nil {
			var writeN int
			splits := c.splitFunc(c.helloBuf.Bytes()[:nHello])
			for _, split := range splits {
				splitN, splitErr := c.Conn.Write(split)
				writeN += splitN
				if splitErr != nil {
					writtenFromB := max(len(b)-(c.helloBuf.Len()-writeN), 0)
					return writtenFromB, BufferedWriteError{contents(c.helloBuf), writeN, nil}
				}
			}
			writtenFromB := len(b) - (c.helloBuf.Len() - nHello)
			c.wroteHello = true
			if c.queuedNoDelayFalse && isTCPConn {
				if err := tcpConn.SetNoDelay(false); err != nil {
					log.Errorf("failed to disable TCP_NODELAY after hello: %v", err)
				}
			}
			c.helloBuf = nil
			return writtenFromB, nil
		} else if !errors.Is(parseErr, io.EOF) {
			return 0, HelloParsingError{contents(c.helloBuf), parseErr}
		}
	}
	return 0, nil
}

func contents(buf *bytes.Buffer) []byte {
	b := make([]byte, buf.Len())
	copy(b, buf.Bytes())
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
