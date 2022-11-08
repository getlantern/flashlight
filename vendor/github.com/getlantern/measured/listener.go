package measured

import (
	"net"
	"time"
)

type listener struct {
	net.Listener
	rateInterval time.Duration
	onFinish     func(Conn)
}

// WrapListener wraps an existing listener with one that will measure accepted
// connections.
func WrapListener(l net.Listener, rateInterval time.Duration, onFinish func(Conn)) net.Listener {
	return &listener{l, rateInterval, onFinish}
}

func (l *listener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		conn = Wrap(conn, l.rateInterval, l.onFinish)
	}
	return conn, err
}
