package quicproxy

import (
	"net"

	"github.com/lucas-clemente/quic-go"
)

type QuicConn struct {
	Sess quic.Connection
	quic.Stream
}

func (qs *QuicConn) Read(b []byte) (int, error) {
	return qs.Stream.Read(b)
}

func (qs *QuicConn) Write(b []byte) (int, error) {
	return qs.Stream.Write(b)
}

func (qs *QuicConn) LocalAddr() net.Addr {
	return qs.Sess.LocalAddr()
}

func (qs *QuicConn) RemoteAddr() net.Addr {
	return qs.Sess.RemoteAddr()
}

func (qs *QuicConn) CloseWrite() error {
	return qs.Sess.CloseWithError(0, "")
}

func (qs *QuicConn) CloseRead() error {
	return qs.Sess.CloseWithError(0, "")
}
