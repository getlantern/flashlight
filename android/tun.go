package android

import (
	"context"
	"io"
	"net"
	"sync/atomic"

	"github.com/getlantern/errors"
	"github.com/getlantern/gotun"
	"github.com/getlantern/netx"
	"golang.org/x/net/proxy"
)

var currentDevice atomic.Value

// Tun2Socks wraps the TUN device identified by fd with a gotun server that does
// the following:
//
// 1. dns packets bound for dnsAddr are rerouted to dnsgrab at dnsGrabAddr
// 2. All other udp packets are routed directly to their destination
// 3. All TCP traffic is routed through the Lantern proxy at the given socksAddr.
//
func Tun2Socks(fd int, socksAddr, dnsAddr, dnsGrabAddr string, mtu int) error {
	dev, err := tun.WrapTunDevice(fd)
	if err != nil {
		return errors.New("Unable to wrap tun device: %v", err)
	}
	socksDialer, err := proxy.SOCKS5("tcp", socksAddr, nil, nil)
	if err != nil {
		return errors.New("Unable to get SOCKS5 dialer: %v", err)
	}
	currentDevice.Store(dev)

	go func() {
		err := tun.Serve(dev, &tun.ServerOpts{
			MTU: mtu,
			DialTCP: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return socksDialer.Dial(network, addr)
			},
			DialUDP: func(ctx context.Context, network, addr string) (*net.UDPConn, error) {
				if addr == dnsAddr {
					// reroute DNS requests to dnsgrab
					addr = dnsGrabAddr
				}
				raddr, err := netx.ResolveUDPAddr(network, addr)
				if err != nil {
					return nil, err
				}
				return netx.DialUDP(network, nil, raddr)
			},
		})
		if err != nil {
			log.Errorf("Error on serving from TUN device: %v", err)
		}
	}()

	return nil
}

// StopTun2Socks stops the current tun device.
func StopTun2Socks() {
	dev := currentDevice.Load()
	if dev != nil {
		dev.(io.Closer).Close()
		currentDevice.Store(nil)
	}
}
