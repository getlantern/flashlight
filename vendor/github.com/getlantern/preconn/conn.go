// Package preconn provides an implementation of net.Conn that allows insertion
// of data before the beginning of the underlying connection.
package preconn

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
)

// Conn is a net.Conn that supports replaying.
type Conn struct {
	net.Conn
	mu           sync.Mutex
	head         io.Reader
	consumedHead bool
}

// Wrap wraps the supplied conn and inserting the given bytes at the head of the
// stream.
func Wrap(conn net.Conn, head []byte) *Conn {
	return WrapReader(conn, bytes.NewReader(head))
}

// WrapReader wraps the supplied conn, reading from 'head' first.
func WrapReader(conn net.Conn, head io.Reader) *Conn {
	return &Conn{conn, sync.Mutex{}, head, false}
}

// Read implements the method from net.Conn and first consumes the head before
// using the underlying connection.
func (conn *Conn) Read(b []byte) (n int, err error) {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.readNoLocking(b)
}

func (conn *Conn) readNoLocking(b []byte) (n int, err error) {
	if conn.consumedHead {
		return conn.Conn.Read(b)
	}
	// "Read conventionally returns what is available instead of waiting for more."
	// - https://golang.org/pkg/io/#Reader
	// Thus if we read off conn.head, we return immediately rather than continuing to conn.Conn.
	// This helps avoid deadlocks - for example, there may be data in conn.head, but none in
	// conn.Conn (and the peer may be waiting for a response to the data in conn.head). We may miss
	// available data in conn.Conn, but the caller can simply call Read again if needed.
	n, err = conn.head.Read(b)
	if errors.Is(err, io.EOF) {
		err = nil
		conn.consumedHead = true
		if n == 0 {
			return conn.readNoLocking(b)
		}
	}
	return
}
