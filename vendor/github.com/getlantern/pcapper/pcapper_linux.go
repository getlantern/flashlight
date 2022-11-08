// Package pcapper provides a facility for continually capturing pcaps at the ip
// level and then dumping those for specific IPs when the time comes.
package pcapper

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/ring"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
	"github.com/hashicorp/golang-lru"
)

var (
	log = golog.LoggerFor("pcapper")

	dumpRequests    = make(chan *dumpRequest, 10000)
	dumpAllRequests = make(chan string, 10)
)

type dumpRequest struct {
	ip      string
	comment string
}

// StartCapturing starts capturing packets from the named network interface. It
// will dump packets into files at <dir>/<ip>.pcap. It will store data for up to
// <numIPs> of the most recently active IPs in memory, and it will store up to
// <packetsPerIP> packets per IP. snapLen specifies the maximum packet length to
// capture and timeout specifies the capture timeout.
func StartCapturing(application string, interfaceName string, dir string, numIPs int, packetsPerIP int, snapLen int, timeout time.Duration) error {
	ifAddrs, err := net.InterfaceAddrs()
	if err != nil {
		return log.Errorf("Unable to determine interface addresses: %v", err)
	}
	localInterfaces := make(map[string]bool, len(ifAddrs))
	for _, ifAddr := range ifAddrs {
		addr := strings.Split(ifAddr.String(), "/")[0] // get rid of CIDR routing prefix
		log.Debugf("Will not save packets for local interface %v", addr)
		localInterfaces[addr] = true
	}

	handle, err := pcap.OpenLive(interfaceName, int32(snapLen), false, timeout)
	if err != nil {
		return log.Errorf("Unable to open %v for packet capture: %v", interfaceName, err)
	}
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())

	buffersByIP, err := lru.New(numIPs)
	if err != nil {
		return log.Errorf("Unable to initialize cache: %v", err)
	}

	getBufferByIP := func(ip string) ring.List {
		_buffer, found := buffersByIP.Get(ip)
		if !found {
			_buffer = ring.NewList(packetsPerIP)
			buffersByIP.Add(ip, _buffer)
		}
		return _buffer.(ring.List)
	}

	capturePacket := func(dstIP string, srcIP string, packet gopacket.Packet) {
		if !localInterfaces[dstIP] {
			getBufferByIP(dstIP).Push(packet)
		} else if !localInterfaces[srcIP] {
			getBufferByIP(srcIP).Push(packet)
		}
	}

	dumpPackets := func(ip string, comment string) error {
		log.Debugf("Attempting to dump pcaps for %v with comment %v", ip, comment)

		defer func() {
			buffersByIP.Remove(ip)
		}()

		buffers := getBufferByIP(ip)
		if buffers.Len() == 0 {
			log.Debugf("No pcaps to dump for %v", ip)
			return nil
		}

		pcapsFileName := filepath.Join(dir, ip+".pcapng")
		pcapsFile, err := os.OpenFile(pcapsFileName, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			if !os.IsNotExist(err) {
				return log.Errorf("Unable to open pcap file %v: %v", pcapsFileName, err)
			}
			pcapsFile, err = os.Create(pcapsFileName)
			if err != nil {
				return log.Errorf("Unable to create pcap file %v: %v", pcapsFileName, err)
			}
		}
		intf := pcapgo.NgInterface{
			Name:                interfaceName,
			OS:                  runtime.GOOS,
			SnapLength:          uint32(snapLen),
			TimestampResolution: 9,
		}
		intf.LinkType = layers.LinkTypeEthernet
		opts := pcapgo.NgWriterOptions{
			SectionInfo: pcapgo.NgSectionInfo{
				Hardware:    runtime.GOARCH,
				OS:          runtime.GOOS,
				Application: application,
				Comment:     comment,
			},
		}
		pcaps, err := pcapgo.NewNgWriterInterface(pcapsFile, intf, opts)
		if err != nil {
			pcapsFile.Close()
			return log.Errorf("Error opening file %v for writing pcaps: %v", pcapsFileName, err)
		}

		dumpPacket := func(dstIP string, srcIP string, packet gopacket.Packet) {
			if dstIP == ip || srcIP == ip {
				ci := packet.Metadata().CaptureInfo
				ci.InterfaceIndex = 0
				err := pcaps.WritePacket(ci, packet.Data())
				if err != nil {
					log.Errorf("Error writing packet to %v: %v", pcapsFileName, err)
				}
			}
		}

		buffers.IterateForward(func(_packet interface{}) bool {
			if _packet == nil {
				// TODO: figure out why we need this guard condition, since we shouldn't
				return false
			}
			packet := _packet.(gopacket.Packet)
			nl := packet.NetworkLayer()
			switch t := nl.(type) {
			case *layers.IPv4:
				dumpPacket(t.DstIP.String(), t.SrcIP.String(), packet)
			case *layers.IPv6:
				dumpPacket(t.DstIP.String(), t.SrcIP.String(), packet)
			}
			return true
		})

		flushErr := pcaps.Flush()
		pcapsFile.Close()
		if flushErr != nil {
			return log.Errorf("Error flushing pcaps to %v", pcapsFileName)
		}
		log.Debugf("Logged pcaps for %v to %v", ip, pcapsFileName)
		return nil
	}

	doDumpRequests := make(chan *dumpRequest, numIPs)
	go func() {
		for {
			select {
			case packet := <-packetSource.Packets():
				nl := packet.NetworkLayer()
				switch t := nl.(type) {
				case *layers.IPv4:
					capturePacket(t.DstIP.String(), t.SrcIP.String(), packet)
				case *layers.IPv6:
					capturePacket(t.DstIP.String(), t.SrcIP.String(), packet)
				}
			case dr := <-dumpRequests:
				// Wait a little bit to make sure we capture the relevant packets
				time.Sleep(timeout * 2)
				doDumpRequests <- dr
			case dr := <-doDumpRequests:
				dumpPackets(dr.ip, dr.comment)
			case comment := <-dumpAllRequests:
				// Wait a little bit to make sure we capture the relevant packets
				time.Sleep(timeout * 2)
				log.Debug("Dumping packets for all IP addresses")
				for _, ip := range buffersByIP.Keys() {
					dumpPackets(ip.(string), comment)
				}
			}
		}
	}()

	return nil
}

// Dump dumps captured packets to/from the given ip to disk.
func Dump(ip string, comment string) {
	select {
	case dumpRequests <- &dumpRequest{ip, comment}:
		// ok
	default:
		log.Errorf("Too many pending dump requests, ignoring request for %v with comment %v", ip, comment)
	}
}

// DumpAll dumps all captured packets for all ips to disk.
func DumpAll(comment string) {
	select {
	case dumpAllRequests <- comment:
		// ok
	default:
		log.Errorf("Too many pending dump requests, ignoring request to dump all with comment %v", comment)
	}

}
