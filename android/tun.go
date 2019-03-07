package android

import (
	"context"
	"io"
	"net"
	"sync"

	"github.com/getlantern/errors"
	"github.com/getlantern/gotun"
	"github.com/getlantern/ipproxy"
	"github.com/getlantern/netx"
	"golang.org/x/net/proxy"
)

var (
	currentDeviceMx sync.Mutex
	currentDevice   io.ReadWriteCloser
	currentIPP      ipproxy.Proxy
)

// Tun2Socks wraps the TUN device identified by fd with an ipproxy server that
// does the following:
//
// 1. dns packets bound for dnsAddr are rerouted to dnsgrab at dnsGrabAddr
// 2. All other udp packets are routed directly to their destination
// 3. All TCP traffic is routed through the Lantern proxy at the given socksAddr.
//
func Tun2Socks(fd int, tunAddr, gwAddr, socksAddr, dnsAddr, dnsGrabAddr string, mtu int) error {
	log.Debugf("Starting tun2socks at %v gw %v connection to socks at %v with dns %v", tunAddr, gwAddr, socksAddr, dnsAddr)
	dev, err := tun.WrapTunDevice(fd, tunAddr, gwAddr)
	if err != nil {
		return errors.New("Unable to wrap tun device: %v", err)
	}
	defer dev.Close()

	socksDialer, err := proxy.SOCKS5("tcp", socksAddr, nil, nil)
	if err != nil {
		return errors.New("Unable to get SOCKS5 dialer: %v", err)
	}

	ipp, err := ipproxy.New(dev, &ipproxy.Opts{
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
		return errors.New("Unable to create ipproxy: %v", err)
	}

	currentDeviceMx.Lock()
	currentDevice = dev
	currentIPP = ipp
	currentDeviceMx.Unlock()

	go func() {
		serveErr := ipp.Serve()
		if serveErr != nil {
			log.Errorf("Error on serving from TUN device: %v", serveErr)
		}
	}()

	return nil
}

// StopTun2Socks stops the current tun device.
func StopTun2Socks() {
	currentDeviceMx.Lock()
	dev := currentDevice
	ipp := currentIPP
	currentDevice = nil
	currentDeviceMx.Unlock()
	if dev != nil {
		log.Debug("Closing TUN device")
		if err := dev.Close(); err != nil {
			log.Errorf("Error closing TUN device: %v", err)
		}
		log.Debug("Closed TUN device")
	}
	if ipp != nil {
		log.Debug("Closing ipproxy")
		if err := currentIPP.Close(); err != nil {
			log.Errorf("Error closing ipproxy: %v", err)
		}
		log.Debug("Closed ipproxy")
	}
}
