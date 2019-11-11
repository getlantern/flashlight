package trafficlog

import (
	"net"
	"runtime"
	"strings"

	"github.com/getlantern/errors"
	"github.com/google/gopacket/pcap"
)

// The network interfaces according to the pcap package are slightly different than the network
// interfaces according to the net package. We need information from both, so we combine the two in
// the networkInterface type.
type networkInterface struct {
	pcapInterface pcap.Interface
	netInterface  net.Interface
	linkType      LinkType
}

// Returns the network interface used to connect to the host.
func networkInterfaceFor(remoteIP net.IP) (*networkInterface, error) {
	linkType := LinkTypeEthernet
	if remoteIP.IsLoopback() && runtime.GOOS != "linux" {
		// This is done to support testing.
		linkType = LinkTypeLoopback
	}

	localIP, err := preferredOutboundIP(remoteIP)
	if err != nil {
		return nil, errors.New("failed to obtain outbound IP: %v", err)
	}

	pcapIface, err := func() (*pcap.Interface, error) {
		pcapIfaces, err := pcap.FindAllDevs()
		if err != nil {
			return nil, errors.New("failed to obtain system interfaces: %v", err)
		}

		for _, iface := range pcapIfaces {
			for _, addr := range iface.Addresses {
				if getIPNet(addr).Contains(localIP) {
					return &iface, nil
				}
				if localIP.IsLoopback() && strings.Contains(strings.ToLower(iface.Description), "loopback") {
					// The Npcap loopback adapter on Windows may not have the loopback IP network in
					// its address space. It is identifiable only through mention of the word
					// "loopback" in its description. Fortunately, this is only really an issue
					// during local testing.
					return &iface, nil
				}
			}
		}
		return nil, errors.New("should connect through %v, but could not find network interface", localIP)
	}()
	if err != nil {
		return nil, err
	}

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
			if ipNet.Contains(localIP) {
				return &networkInterface{*pcapIface, iface, linkType}, nil
			}
		}
	}
	return nil, errors.New("should connect through %v, but could not find network interface", localIP)
}

// The pcap and net packages sometimes report different names for the interfaces. Functions in the
// pcap package require the name reported by the pcap package.
func (ni networkInterface) pcapName() string {
	return ni.pcapInterface.Name
}

// As noted above, the pcap and net packages sometimes report different names for the interfaces.
// While the pcap name is needed sometimes, the net package's name is often the more conventional.
func (ni networkInterface) name() string {
	return ni.netInterface.Name
}

func (ni networkInterface) mtu() int {
	return ni.netInterface.MTU
}

func (ni networkInterface) index() int {
	return ni.netInterface.Index
}

func preferredOutboundIP(remoteIP net.IP) (net.IP, error) {
	// Note: the choice of port below does not actually matter. It just needs to be non-zero.
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: remoteIP, Port: 999})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP, nil
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
	_, network, err := net.ParseCIDR(addr)
	if err == nil {
		return network, nil
	}
	ip := net.ParseIP(addr)
	if ip == nil {
		return nil, err
	}
	if ip.To4() != nil {
		return &net.IPNet{IP: ip.To4(), Mask: net.CIDRMask(0, 32)}, nil
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(0, 128)}, nil
}
