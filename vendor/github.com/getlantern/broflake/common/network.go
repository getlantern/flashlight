package common

import (
	"net"
	"time"

	"github.com/lucas-clemente/quic-go"
)

var QUICCfg = quic.Config{
	MaxIncomingStreams:    int64(2 << 16),
	MaxIncomingUniStreams: int64(2 << 16),
	MaxIdleTimeout:        16 * time.Second,
	KeepAlivePeriod:       8 * time.Second,
}

type DebugAddr string

func (a DebugAddr) Network() string {
	return string(a)
}

func (a DebugAddr) String() string {
	return string(a)
}

type QUICStreamNetConn struct {
	quic.Stream
}

func (c QUICStreamNetConn) LocalAddr() net.Addr {
	return DebugAddr("DEBUG NELSON WUZ HERE")
}

func (c QUICStreamNetConn) RemoteAddr() net.Addr {
	return DebugAddr("DEBUG NELSON WUZ HERE")
}

func IsPublicAddr(addr net.IP) bool {
	return !addr.IsPrivate() && !addr.IsUnspecified() && !addr.IsLoopback()
}
