// TODO <31-01-2023, soltzen> This is identical to the one in Flashlight. Coallesce them before merging the PRs
package prefix

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/getlantern/golog"
)

var log = golog.LoggerFor("prefix")

type ReadPrefixConn struct {
	net.Conn
	ID string

	prefixSize int
	once       sync.Once
}

func NewReadPrefixConn(ID string, conn net.Conn, prefixSize int) *ReadPrefixConn {
	return &ReadPrefixConn{
		ID:         ID,
		Conn:       conn,
		prefixSize: prefixSize,
	}
}

func (p *ReadPrefixConn) Read(b []byte) (n int, err error) {
	p.once.Do(func() {
		prefixBuf := make([]byte, p.prefixSize)
		_, err = p.Conn.Read(prefixBuf)
		if err == nil {
			log.Debugf("ReadPrefixConn %s: Read prefix (%s) of size %d", p.ID, prefixBuf, p.prefixSize)
		}
	})

	// Check the error from the prefix read, if any
	if err != nil {
		return 0, fmt.Errorf(
			"Unable to read prefix of size %d: %v", p.prefixSize, err)
	}

	return p.Conn.Read(b)
}

func (p *ReadPrefixConn) Write(b []byte) (n int, err error) {
	return p.Conn.Write(b)
}

func (p *ReadPrefixConn) Close() error {
	return p.Conn.Close()
}

func (p *ReadPrefixConn) LocalAddr() net.Addr {
	return p.Conn.LocalAddr()
}

func (p *ReadPrefixConn) RemoteAddr() net.Addr {
	return p.Conn.RemoteAddr()
}

func (p *ReadPrefixConn) SetDeadline(t time.Time) error {
	return p.Conn.SetDeadline(t)
}

func (p *ReadPrefixConn) SetReadDeadline(t time.Time) error {
	return p.Conn.SetReadDeadline(t)
}

func (p *ReadPrefixConn) SetWriteDeadline(t time.Time) error {
	return p.Conn.SetWriteDeadline(t)
}
