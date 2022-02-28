package quicproxy

import (
	"net"

	"github.com/getlantern/golog"
	"github.com/lucas-clemente/quic-go"
)

var (
	log = golog.LoggerFor("replica-p2p-quicproxy")
)

type QuicConn struct {
	Sess quic.Session
	quic.Stream
}

func (qs *QuicConn) LocalAddr() net.Addr {
	return qs.Sess.LocalAddr()
}

func (qs *QuicConn) RemoteAddr() net.Addr {
	return qs.Sess.RemoteAddr()
}
