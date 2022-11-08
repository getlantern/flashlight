package packetcounter

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/getlantern/golog"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

const (
	snaplen int32 = 18 + 60 + 60 // The total of maximum length of Ethernet, IP and TCP headers
)

var (
	log = golog.LoggerFor("packetcounter")
)

// ReportFN is a callback to report how many sentDataPackets have been sent over a TCP
// connection made from the clientAddr and of which how many are
// retransmissions. It gets called when the connection terminates.
type ReportFN func(clientAddr string, sentDataPackets, retransmissions, consecRetransmissions int)

// Track keeps capturing all TCP replies from the listening port on the
// interface, and reports when the connection terminates.
func Track(interfaceName, listenPort string, report ReportFN) {
	ips, err := interfaceAddrs(interfaceName)
	if err != nil {
		log.Errorf("Unable to get IPs bound to %v: %v", interfaceName, err)
		return
	}
	filter, err := composeBPF(ips, listenPort)
	if err != nil {
		log.Errorf("Unable to compose BPF %v for packet capture: %v", interfaceName, err)
		return
	}
	handle, err := pcap.OpenLive(interfaceName, snaplen, false /*promisc*/, pcap.BlockForever)
	if err != nil {
		log.Errorf("Unable to open %v for packet capture: %v", interfaceName, err)
		return
	}
	if err := handle.SetBPFFilter(filter); err != nil {
		log.Errorf("Unable to set BPF filter '%v': %v", filter, err)
		return
	}

	// Map of the string form of the TCPAddr to the counters
	flows := map[string]struct {
		lastSeq               uint32
		sentDataPackets       int
		retransmissions       int
		consecRetransmissions int
	}{}
	var ether layers.Ethernet
	var ip4 layers.IPv4
	var ip6 layers.IPv6
	var tcp layers.TCP
	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet,
		&ether, &ip4, &ip6, &tcp)
	decoded := make([]gopacket.LayerType, 0, 4)
	chDeleteFlow := make(chan string)

	for {
		select {
		case key := <-chDeleteFlow:
			delete(flows, key)
		default:
		}
		data, _, err := handle.ZeroCopyReadPacketData()
		if err != nil {
			log.Debugf("error getting packet: %v", err)
			continue
		}
		// error is expected because we don't decode TLS. Ranging over decoded
		// will get the correct result.
		_ = parser.DecodeLayers(data, &decoded)
		var dst net.TCPAddr
		var payloadLen uint16
		var tcpDecoded bool
		for _, typ := range decoded {
			switch typ {
			case layers.LayerTypeIPv4:
				dst.IP = ip4.DstIP
				payloadLen = ip4.Length - uint16(ip4.IHL<<2)
			case layers.LayerTypeIPv6:
				dst.IP = ip6.DstIP
				payloadLen = ip6.Length
			case layers.LayerTypeTCP:
				tcpDecoded = true
			}
		}
		if !tcpDecoded {
			log.Error("TCP packet is expected but not seen")
			continue
		}
		dataLen := payloadLen - uint16(tcp.DataOffset<<2)
		// skip pure ACKs to avoid miscounting them as retransmissions
		if dataLen == 0 && !tcp.RST && !tcp.FIN {
			continue
		}
		dst.Port = int(tcp.DstPort)
		key := dst.String()
		if tcp.FIN || tcp.RST {
			flow := flows[key]
			if flow.sentDataPackets > 0 {
				report(key, flow.sentDataPackets, flow.retransmissions, flow.consecRetransmissions)
			}
			// Delay removing the flow to prevent retransmissions of previous
			// packets from creating dangling entries which never get removed.
			time.AfterFunc(10*time.Minute, func() {
				chDeleteFlow <- key
			})
			continue
		}
		flow := flows[key]
		if tcp.Seq > flow.lastSeq {
			flow.lastSeq = tcp.Seq
			flow.sentDataPackets++
			flow.consecRetransmissions = 0
		} else {
			flow.retransmissions++
			flow.consecRetransmissions++
		}
		flows[key] = flow
	}
}

func interfaceAddrs(interfaceName string) ([]net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, i := range interfaces {
		if i.Name == interfaceName {
			addrs, err := i.Addrs()
			if err != nil {
				return nil, err
			}
			var ips []net.IP
			for _, addr := range addrs {
				switch v := addr.(type) {
				case *net.IPNet:
					ips = append(ips, v.IP)
				// below are unlikely to happen, but just to be safe
				case *net.IPAddr:
					ips = append(ips, v.IP)
				default:
					return nil, fmt.Errorf("unrecognized address type %T on interface %s", v, i.Name)
				}
			}
			return ips, nil
		}
	}
	return nil, errors.New("interface not found")
}

func composeBPF(ips []net.IP, listenPort string) (string, error) {
	switch len(ips) {
	case 0:
		return "", errors.New("no address is configured on interface")
	case 1:
		return fmt.Sprintf("tcp and src port %s and src host %s", listenPort, ips[0].String()), nil
	default:
		str := fmt.Sprintf("tcp and src port %s and (src host %s", listenPort, ips[0].String())
		for _, ip := range ips[1:] {
			str += " or src host " + ip.String()
		}
		str += ")"
		return str, nil
	}
}
