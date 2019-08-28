package diagnostics

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/chained"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
)

const (
	// TODO: perhaps this should be configurable
	captureDuration = 10 * time.Second

	// Warning: do not set to >= 1 second: https://github.com/google/gopacket/issues/499
	packetReadTimeout = 100 * time.Millisecond
)

// ErrorsMap represents multiple errors. ErrorsMap implements the error interface.
type ErrorsMap map[string]error

func (em ErrorsMap) Error() string {
	keys := []string{}
	for k := range em {
		keys = append(keys, k)
	}
	return fmt.Sprintf("errors for %s", strings.Join(keys, ", "))
}

// CaptureProxyTraffic generates a pcap file for each proxy in the input map. These files are saved
// in outputDir and named using the keys in proxiesMap.
//
// If an error is returned, it will be of type ErrorsMap. The keys of the map will be the keys in
// proxiesMap. If no error occurred for a given proxy, it will have no entry in the returned map.
//
// Expects tshark to be installed, otherwise returns errors.
func CaptureProxyTraffic(proxiesMap map[string]*chained.ChainedServerInfo, outputDir string) error {
	// TODO: one file for all proxy traffic is probably more convenient

	type captureError struct {
		proxyName string
		err       error
	}

	wg := new(sync.WaitGroup)
	captureErrors := make(chan captureError, len(proxiesMap))
	for proxyName, proxyInfo := range proxiesMap {
		wg.Add(1)
		go func(pName, pAddr string) {
			defer wg.Done()
			err := writeCapture(pAddr, filepath.Join(outputDir, fmt.Sprintf("%s.pcap", pName)), captureDuration)
			if err != nil {
				captureErrors <- captureError{pName, err}
			}
		}(proxyName, proxyInfo.Addr)
	}
	wg.Wait()
	close(captureErrors)

	errorsMap := ErrorsMap{}
	for capErr := range captureErrors {
		errorsMap[capErr.proxyName] = capErr.err
	}
	if len(errorsMap) == 0 {
		return nil
	}
	return errorsMap
}

func writeCapture(addr, outputFile string, duration time.Duration) error {
	f, err := os.Create(outputFile)
	if err != nil {
		return errors.New("failed to open file for writing: %v", err)
	}
	defer f.Close()

	remoteHost, remotePort, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("malformed address: %v", err)
	}

	remoteIPs, err := net.LookupIP(remoteHost)
	if err != nil {
		return errors.New("failed to find IP for host: %v", err)
	}
	if len(remoteIPs) < 1 {
		return errors.New("failed to resolve host")
	}
	remoteIP := remoteIPs[0]

	localIP, err := preferredOutboundIP(remoteIP)
	if err != nil {
		return errors.New("failed to obtain outbound IP: %v", err)
	}

	iface, err := interfaceFor(localIP)
	if err != nil {
		return errors.New("failed to obtain interface: %v", err)
	}

	mtu, err := getMTU(*iface)
	if err != nil {
		return errors.New("failed to obtain interface MTU: %v", err)
	}

	// TODO: test on linux
	linkType := layers.LinkTypeEthernet
	if remoteIP.IsLoopback() && runtime.GOOS != "linux" {
		// This is done to support testing.
		linkType = layers.LinkTypeNull
		// TODO: should this ever be LinkTypeLoop?
	}

	pcapW := pcapgo.NewWriter(f)
	if err := pcapW.WriteFileHeader(uint32(mtu), linkType); err != nil {
		return errors.New("failed to write header to capture file: %v", err)
	}

	handle, err := pcap.OpenLive(iface.Name, int32(mtu), false, packetReadTimeout)
	if err != nil {
		return errors.New("failed to open capture handle: %v", err)
	}
	defer handle.Close()

	network := "ip"
	if remoteIP.To4() == nil {
		network = "ip6"
	}
	bpf := fmt.Sprintf(
		"(%s) or (%s)",
		fmt.Sprintf("%s dst %v and dst port %s", network, remoteIP, remotePort),
		fmt.Sprintf("%s src %v and src port %s", network, remoteIP, remotePort),
	)
	if err := handle.SetBPFFilter(bpf); err != nil {
		return errors.New("failed to set capture filter: %v", err)
	}

	layerType := layers.LayerTypeEthernet
	if remoteIP.IsLoopback() && runtime.GOOS != "linux" {
		// This is done to support testing.
		layerType = layers.LayerTypeLoopback
	}

	pktSrc := gopacket.NewPacketSource(handle, layerType).Packets()
	pktWriteErrors := []error{}
	for {
		select {
		case pkt := <-pktSrc:
			if err := pcapW.WritePacket(pkt.Metadata().CaptureInfo, pkt.Data()); err != nil {
				pktWriteErrors = append(pktWriteErrors, err)
			}
		case <-time.After(captureDuration):
			if len(pktWriteErrors) == 0 {
				return nil
			}
			return errors.New("%d errors writing packets; first error: %v", len(pktWriteErrors), pktWriteErrors[0])
		}
	}
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

func interfaceFor(ip net.IP) (*pcap.Interface, error) {
	ifaces, err := pcap.FindAllDevs()
	if err != nil {
		return nil, errors.New("failed to obtain system interfaces: %v", err)
	}

	for _, iface := range ifaces {
		for _, addr := range iface.Addresses {
			if getIPNet(addr).Contains(ip) {
				return &iface, nil
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

func getMTU(iface pcap.Interface) (int, error) {
	// We use the interface's unicast addresses to associate with a net.Interface, then obtain the
	// MTU from the net.Interface. This is pretty roundabout, but the MTU is not exposed on
	// pcap.Interface. At the same time, pcap.Interface.Name can differ from net.Interface.Name and
	// the pcap package requires the name given to pcap.Interface.

	netIfaces, err := net.Interfaces()
	if err != nil {
		return 0, errors.New("failed to read interface information from net pkg: %v", err)
	}
	for _, netIface := range netIfaces {
		addrs, err := netIface.Addrs()
		if err != nil {
			return 0, errors.New("failed to read addresses from net.Interface: %v", err)
		}
		for _, candidateAddr := range addrs {
			candidateIPNet, ok := candidateAddr.(*net.IPNet)
			if !ok {
				continue
			}
			for _, ifaceAddr := range iface.Addresses {
				if bytes.Equal(ifaceAddr.IP, candidateIPNet.IP) && bytes.Equal(ifaceAddr.Netmask, candidateIPNet.Mask) {
					return netIface.MTU, nil
				}
			}
		}
	}
	return 0, errors.New("unable to find interface metadata")
}
