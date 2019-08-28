package diagnostics

import (
	"net"
	"strconv"
	"strings"

	"github.com/getlantern/errors"
	"github.com/google/gopacket/pcap"
)

// The pcap package requires the interface name provided by pcap.Interface (which can differ from
// that given to net.Interface). We also need the MTU, which is not provided by pcap.Interface. The
// networkInterface type combines this information.
type networkInterface struct {
	pcap.Interface
	mtu int
}

func networkInterfaceFor(ip net.IP) (*networkInterface, error) {
	pcapIface, err := func() (*pcap.Interface, error) {
		pcapIfaces, err := pcap.FindAllDevs()
		if err != nil {
			return nil, errors.New("failed to obtain system interfaces: %v", err)
		}

		for _, iface := range pcapIfaces {
			for _, addr := range iface.Addresses {
				if getIPNet(addr).Contains(ip) {
					return &iface, nil
				}
				if ip.IsLoopback() && strings.Contains(strings.ToLower(iface.Description), "loopback") {
					// The Npcap loopback adapter on Windows may not have the loopback IP network in
					// its address space. It is identifiable (reliably) only through mention of the
					// word "loopback" in its description. Fortunately, this is only really an issue
					// during local testing.
					return &iface, nil
				}
			}
		}
		return nil, errors.New("no network interface for %v", ip)
	}()
	if err != nil {
		return nil, err
	}

	// We've found the network interface according to the pcap package. Unfortunately, the pcap
	// package does not expose the interface MTU, so we need to go through a similar process with
	// the net package.

	netIfaces, err := net.Interfaces()
	if err != nil {
		return nil, errors.New("failed to obtain system interfaces: %v", err)
	}
	for _, iface := range netIfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, errors.New("failed to obtain addresses for %s: %v", iface.Name, err)
		}
		for _, addr := range addrs {
			ipNet, err := parseNetwork(addr.String())
			if err != nil {
				return nil, errors.New("failed to parse interface address %s as IP network: %v", addr.String(), err)
			}
			if ipNet.Contains(ip) {
				return &networkInterface{*pcapIface, iface.MTU}, nil
			}
		}
	}
	return nil, errors.New("no network interface for %v", ip)
}

func getIPNet(addr pcap.InterfaceAddress) *net.IPNet {
	ipNet := net.IPNet{IP: addr.IP, Mask: addr.Netmask}
	if ipNet.Mask != nil {
		return &ipNet
	}
	if ipNet.IP.To4() != nil {
		ipNet.Mask = net.CIDRMask(0, 32)
	} else {
		ipNet.Mask = net.CIDRMask(0, 128)
	}
	return &ipNet
}

// Parses a network of addresses like 127.0.0.1/8. Inputs like 127.0.0.1 are valid and will be
// interpreted as equivalent to 127.0.0.1/0.
func parseNetwork(addr string) (*net.IPNet, error) {
	splits := strings.Split(addr, "/")

	var (
		ip       net.IP
		maskOnes int
		err      error
	)
	switch len(splits) {
	case 1:
		ip = net.ParseIP(addr)
		maskOnes = 0
	case 2:
		ip = net.ParseIP(splits[0])
		maskOnes, err = strconv.Atoi(splits[1])
		if err != nil {
			return nil, errors.New("expected integer after '/' character, found %s", splits[1])
		}
	default:
		return nil, errors.New("expected one or zero '/' characters in address, found %d", len(splits))
	}

	if ip == nil {
		return nil, errors.New("failed to parse network number as IP address")
	}
	var mask net.IPMask
	if ip.To4() != nil {
		mask = net.CIDRMask(maskOnes, 32)
	} else {
		mask = net.CIDRMask(maskOnes, 128)
	}
	return &net.IPNet{IP: ip, Mask: mask}, nil
}
