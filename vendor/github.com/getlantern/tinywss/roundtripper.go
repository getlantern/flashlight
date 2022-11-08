package tinywss

import (
	"crypto/tls"
	"net"
	"net/http"

	"github.com/getlantern/bufconn"
	"github.com/getlantern/netx"
)

type DialFN func(network, addr string) (net.Conn, error)

var defaultDial DialFN = TLSDialFN(nil)

func TLSDialFN(tlsConf *tls.Config) DialFN {
	return func(network, addr string) (net.Conn, error) {
		conn, err := netx.Dial(network, addr)
		if err != nil {
			return nil, err
		}
		tlsConn := tls.Client(conn, tlsConf)
		if err = tlsConn.Handshake(); err != nil {
			return nil, err
		}
		return tlsConn, nil
	}
}

// creates a new default RoundTripHijacker
func NewRoundTripper(dial DialFN) *roundTripHijacker {
	if dial == nil {
		dial = defaultDial
	}
	return &roundTripHijacker{
		dial: dial,
	}
}

var _ RoundTripHijacker = &roundTripHijacker{}

// this is the default RoundTripHijacker used for Clients
type roundTripHijacker struct {
	dial DialFN
}

func (rt *roundTripHijacker) RoundTripHijack(req *http.Request) (*http.Response, net.Conn, error) {
	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = "443"
	}
	addr := net.JoinHostPort(host, port)

	conn, err := rt.dial("tcp", addr)
	if err != nil {
		return nil, nil, err
	}

	if err = req.Write(conn); err != nil {
		conn.Close()
		return nil, nil, err
	}

	bconn := bufconn.Wrap(conn)
	res, err := http.ReadResponse(bconn.Head(), req)
	if err != nil {
		bconn.Close()
		return nil, nil, err
	}

	return res, bconn, nil
}
