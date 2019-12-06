package vpn

import (
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/ipproxy"
	"github.com/getlantern/netx"
)

var (
	log = golog.LoggerFor("vpn")
)

const (
	mtu = 65535
)

type vpn struct {
	internetGateway   string
	dev               io.ReadWriteCloser
	proxy             ipproxy.Proxy
	dialer            net.Dialer
	protectedIPs      map[string]bool
	protectedIPsMutex sync.Mutex
}

// Enable enables the VPN and returns a function that can be used to close the VPN when finished
func Enable(socksAddr, internetGateway, tunDeviceName, tunAddr, tunMask string) (func() error, error) {
	dev, err := ipproxy.TUNDevice(tunDeviceName, tunAddr, tunMask, mtu)
	if err != nil {
		return nil, errors.New("error opening TUN device: %v", err)
	}

	socksDialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		dev.Close()
		return nil, errors.New("Unable to create SOCKS5 dialer: %v", err)
	}

	proxy, err := ipproxy.New(dev, &ipproxy.Opts{
		IdleTimeout:   70 * time.Second,
		StatsInterval: 30 * time.Second,
		DialTCP: func(ctx context.Context, network, addr string) (net.Conn, error) {
			log.Debugf("Dialing %v %v with SOCKS proxy at %v", network, addr, socksAddr)
			return socksDialer.Dial(network, addr)
		},
		DialUDP: func(ctx context.Context, network, addr string) (*net.UDPConn, error) {
			log.Debugf("Dialing %v %v directly", network, addr)
			conn, err := netx.DialContext(ctx, network, addr)
			if conn != nil {
				return conn.(*net.UDPConn), err
			}
			return nil, err
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	v := &vpn{
		internetGateway: internetGateway,
		dev:             dev,
		proxy:           proxy,
		protectedIPs:    make(map[string]bool),
	}

	v.configureNetx()
	go func() {
		log.Debug("About to start serving with ipproxy")
		if serveErr := proxy.Serve(); serveErr != nil {
			v.Close()
			log.Error(serveErr)
		}
	}()

	return v.Close, nil
}

func (v *vpn) configureNetx() {
	netx.OverrideDial(func(ctx context.Context, network string, addr string) (net.Conn, error) {
		host, port, splitErr := net.SplitHostPort(addr)
		if splitErr != nil {
			return nil, log.Errorf("unable to determine host to protect: %v", splitErr)
		}
		ips, lookupErr := net.LookupIP(host)
		if lookupErr != nil {
			return nil, log.Errorf("unable to lookup IPs for %v in order to protect connection: %v", host, lookupErr)
		}
		var ip net.IP
		foundIP := false
		for _, _ip := range ips {
			// only support IPv4 at the moment
			if _ip.To4() != nil {
				ip = _ip
				foundIP = true
				break
			}
		}
		if !foundIP {
			return nil, log.Errorf("didn't resolve any IPv4 IPs for %v, can't protect outbound connection", host)
		}
		if protectErr := v.protect(ip); protectErr != nil {
			return nil, log.Error(protectErr)
		}
		return v.dialer.DialContext(ctx, network, fmt.Sprintf("%v:%v", ip, port))
	})

	netx.OverrideDialUDP(func(network string, laddr, raddr *net.UDPAddr) (*net.UDPConn, error) {
		if protectErr := v.protect(raddr.IP); protectErr != nil {
			return nil, protectErr
		}
		return net.DialUDP(network, laddr, raddr)
	})

}

func (v *vpn) protect(ip net.IP) error {
	ipString := ip.String()
	if ip.To4() == nil {
		return errors.New("Protecting IPv6 address unsupported: %v", ipString)
	}

	v.protectedIPsMutex.Lock()
	defer v.protectedIPsMutex.Unlock()
	if !v.protectedIPs[ipString] {
		out, addErr := exec.Command("route", "add", ipString, "gw", v.internetGateway).CombinedOutput()
		if addErr != nil {
			if !strings.Contains(string(out), "File exists") {
				return errors.New("unable to protect route to %v: %v", ipString, string(out))
			}
		}
		v.protectedIPs[ipString] = true
	}

	return nil
}

func (v *vpn) Close() error {
	log.Debug("Closing TUN device")
	v.dev.Close()
	log.Debug("Closed TUN device")
	v.proxy.Close()

	log.Debug("Deleting bypass routes")
	v.protectedIPsMutex.Lock()
	defer v.protectedIPsMutex.Unlock()

	for ip := range v.protectedIPs {
		if deleteErr := exec.Command("route", "delete", ip).Run(); deleteErr != nil {
			log.Errorf("Error deleting route for %v: %v", ip, deleteErr)
		} else {
			delete(v.protectedIPs, ip)
		}
	}
	log.Debug("Done deleting bypass routes")

	return nil
}
