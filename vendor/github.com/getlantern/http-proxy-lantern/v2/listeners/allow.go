package listeners

import (
	"net"
	"reflect"
)

type allowingListener struct {
	wrapped net.Listener
	allow   func(string) bool
}

func NewAllowingListener(l net.Listener, allow func(string) bool) net.Listener {
	return &allowingListener{l, allow}
}

func (l *allowingListener) Accept() (net.Conn, error) {
	conn, err := l.wrapped.Accept()
	if err != nil {
		return conn, err
	}

	ip := ""
	remoteAddr := conn.RemoteAddr()
	switch addr := remoteAddr.(type) {
	case *net.TCPAddr:
		ip = addr.IP.String()
	case *net.UDPAddr:
		ip = addr.IP.String()
	default:
		log.Errorf("Remote addr %v is of unknown type %v, unable to determine IP", remoteAddr, reflect.TypeOf(remoteAddr))
		return conn, err
	}
	if !l.allow(ip) {
		conn.Close()
		// Note - we don't return an error, because that causes http.Server to stop
		// serving.
	}

	return conn, err
}

func (l *allowingListener) Close() error {
	return l.wrapped.Close()
}

func (l *allowingListener) Addr() net.Addr {
	return l.wrapped.Addr()
}
