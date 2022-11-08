package kcpwrapper

import (
	"net"

	"github.com/golang/snappy"
)

type snappyConn struct {
	net.Conn
	w *snappy.Writer
	r *snappy.Reader
}

func (c *snappyConn) Read(p []byte) (n int, err error) {
	return c.r.Read(p)
}

func (c *snappyConn) Write(p []byte) (n int, err error) {
	n, err = c.w.Write(p)
	if err != nil {
		return n, err
	}
	err = c.w.Flush()
	return n, err
}

func wrapSnappy(conn net.Conn) net.Conn {
	c := &snappyConn{Conn: conn}
	c.w = snappy.NewBufferedWriter(conn)
	c.r = snappy.NewReader(conn)
	return c
}
