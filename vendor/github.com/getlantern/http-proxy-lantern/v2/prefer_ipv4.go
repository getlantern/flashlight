package proxy

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"
)

// preferIPV4Dialer returns a function with same signature as net.Dial, but
// always dials the host to its IPv4 address, unless it's already in IP address
// form.
func preferIPV4Dialer(timeout time.Duration) func(ctx context.Context, network, hostport string) (net.Conn, error) {
	return func(ctx context.Context, network, hostport string) (net.Conn, error) {
		tcpAddr, err := tcpAddrPrefer4(hostport)
		if err != nil {
			return nil, err
		}

		dialer := net.Dialer{
			Deadline: time.Now().Add(timeout),
		}
		conn, err := dialer.DialContext(ctx, "tcp4", tcpAddr.String())
		if err != nil {
			var e *net.AddrError
			// if this is a network address error, we will retry with the specified network instead (tcp6 most likely)
			if errors.As(err, &e) {
				conn, err = dialer.DialContext(ctx, network, hostport)
			}
		}
		return conn, err
	}
}

func tcpAddrPrefer4(hostport string) (*net.TCPAddr, error) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	if ip != nil {
		// if host is in IP address form, use it as is
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, err
		}
		return &net.TCPAddr{IP: ip, Port: p, Zone: ""}, nil
	}
	return net.ResolveTCPAddr("tcp4", hostport)
}
