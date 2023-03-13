// PrefixTCPListener is a TCPListener that expects a prefix of a given size in
// the first message after the TCP handshake. The prefix is discarded and the
// rest of the message is returned.
//
// This is mainly useful for testing. Don't use it for other things.
package chained

import (
	"fmt"
	"net"
	"sync"

	"github.com/getlantern/errors"
)

type PrefixTCPListener struct {
	*net.TCPListener

	prefixSize int
}

func NewPrefixTCPListener(l *net.TCPListener, prefixSize int) *PrefixTCPListener {
	return &PrefixTCPListener{TCPListener: l, prefixSize: prefixSize}
}

func (l *PrefixTCPListener) Accept() (net.Conn, error) {
	conn, err := l.TCPListener.Accept()
	if err != nil {
		return nil, err
	}
	return &PrefixTCPConn{Conn: conn, prefixSize: l.prefixSize}, nil
}

func (l *PrefixTCPListener) Close() error {
	return l.TCPListener.Close()
}

func (l *PrefixTCPListener) Addr() net.Addr {
	return l.TCPListener.Addr()
}

type PrefixTCPConn struct {
	net.Conn

	prefixSize     int
	prefixReadOnce sync.Once
}

func (c *PrefixTCPConn) Read(b []byte) (int, error) {
	prefixReadSize := 0
	var err error
	c.prefixReadOnce.Do(func() {
		if len(b) < c.prefixSize {
			err = errors.New("buffer too small")
			return
		}

		// Read the prefix
		prefixBuf := make([]byte, c.prefixSize)
		var n int
		n, err = c.Conn.Read(prefixBuf)
		if err != nil {
			err = fmt.Errorf("unable to read prefix of size [%d]: %v",
				c.prefixSize, err)
			return
		}
		prefixReadSize = n
	})
	if err != nil {
		return 0, err
	}

	// Read the rest of the message
	n, err := c.Conn.Read(b)
	if err != nil {
		return n, err
	}

	return n + prefixReadSize, nil
}
