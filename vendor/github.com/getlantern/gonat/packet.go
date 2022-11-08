// Checksum computations based on logic in github.com/google/gopacket
//
// Copyright 2012 Google, Inc. All rights reserved.
// Copyright 2009-2011 Andreas Krennmair. All rights reserved.

package gonat

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"

	"github.com/getlantern/errors"
	"github.com/oxtoacart/bpool"
)

// TCPFlags are the different flags supported in the TCP header
const (
	TCPFlagRST = 0x04

	tcpHeaderLength = 24
	udpHeaderLength = 8
)

var (
	networkByteOrder = binary.BigEndian
)

func parseIPPacket(raw bpool.ByteSlice) (*IPPacket, error) {
	ipVersion := uint8(raw.Bytes()[0]) >> 4
	if ipVersion != 4 {
		return nil, errors.New("Unsupported ip protocol version: %v", ipVersion)
	}

	pkt := &IPPacket{Raw: raw, IPVersion: ipVersion}
	return pkt.parseV4()
}

type IPPacket struct {
	Raw       bpool.ByteSlice
	IPVersion uint8
	IPProto   uint8
	SrcAddr   *net.IPAddr
	DstAddr   *net.IPAddr
	Header    []byte
	Payload   []byte
}

func (pkt *IPPacket) parseV4() (*IPPacket, error) {
	raw := pkt.Raw.Bytes()
	ihl := uint8(raw[0]) & 0x0F
	length := networkByteOrder.Uint16(raw[2:4])
	if length < 20 {
		return pkt, errors.New("Invalid (too small) IP length (%d < 20)", length)
	} else if ihl < 5 {
		return pkt, errors.New("Invalid (too small) IP header length (%d < 5)", ihl)
	} else if int(ihl*4) > int(length) {
		return pkt, errors.New("Invalid IP header length > IP length (%d > %d)", ihl, length)
	} else if int(ihl)*4 > len(raw) {
		return pkt, errors.New("Not all IP header bytes available")
	}

	pkt.Header = raw[:ihl*4]
	pkt.Payload = raw[ihl*4:]
	pkt.IPProto = uint8(pkt.Header[9])
	pkt.SrcAddr = &net.IPAddr{IP: net.IP(pkt.Header[12:16])}
	pkt.DstAddr = &net.IPAddr{IP: net.IP(pkt.Header[16:20])}

	payloadLength := len(pkt.Payload)
	switch pkt.IPProto {
	case syscall.IPPROTO_TCP:
		if payloadLength < tcpHeaderLength {
			return pkt, errors.New("TCP Packet too short: (%d < %d)", payloadLength, tcpHeaderLength)
		}
	case syscall.IPPROTO_UDP:
		if payloadLength < udpHeaderLength {
			return pkt, errors.New("UDP Packet too short: (%d < %d)", payloadLength, udpHeaderLength)
		}
	}

	return pkt, nil
}

// HasTCPFlag returns true if the packet is a TCP packet that has the given flag set.
func (pkt *IPPacket) HasTCPFlag(flag uint8) bool {
	return pkt.IPProto == syscall.IPPROTO_TCP && pkt.Payload[13]&flag != 0
}

func (pkt *IPPacket) SetSource(addr Addr) {
	ip := addr.IP()
	copy(pkt.Header[12:16], ip)
	networkByteOrder.PutUint16(pkt.Payload[0:2], addr.Port)
	pkt.SrcAddr = &net.IPAddr{IP: ip}
}

func (pkt *IPPacket) SetDest(addr Addr) {
	ip := addr.IP()
	copy(pkt.Header[16:20], ip)
	networkByteOrder.PutUint16(pkt.Payload[2:4], addr.Port)
	pkt.DstAddr = &net.IPAddr{IP: ip}
}

func (pkt *IPPacket) ipChecksum() uint16 {
	return networkByteOrder.Uint16(pkt.Header[10:])
}

func (pkt *IPPacket) recalcIPChecksum() {
	// Clear checksum bytes
	pkt.Header[10] = 0
	pkt.Header[11] = 0

	// Compute checksum
	var csum uint32
	for i := 0; i < len(pkt.Header); i += 2 {
		csum += uint32(pkt.Header[i]) << 8
		csum += uint32(pkt.Header[i+1])
	}
	for {
		// Break when sum is less or equals to 0xFFFF
		if csum <= 65535 {
			break
		}
		// Add carry to the sum
		csum = (csum >> 16) + uint32(uint16(csum))
	}
	// Flip all the bits
	networkByteOrder.PutUint16(pkt.Header[10:], ^uint16(csum))
}

func (pkt *IPPacket) tcpChecksum() uint16 {
	return networkByteOrder.Uint16(pkt.Payload[16:])
}

func (pkt *IPPacket) udpChecksum() uint16 {
	return networkByteOrder.Uint16(pkt.Payload[6:])
}

func (pkt *IPPacket) recalcChecksum() {
	switch pkt.IPProto {
	case syscall.IPPROTO_TCP:
		pkt.recalcTCPChecksum()
	case syscall.IPPROTO_UDP:
		pkt.recalcUDPChecksum()
	}
	pkt.recalcIPChecksum()
}

func (pkt *IPPacket) recalcTCPChecksum() {
	pkt.recalcTransportChecksum(16)
}

func (pkt *IPPacket) recalcUDPChecksum() {
	pkt.recalcTransportChecksum(6)
}

func (pkt *IPPacket) recalcTransportChecksum(csumIdx int) {
	// Clear checksum bytes
	pkt.Payload[csumIdx] = 0
	pkt.Payload[csumIdx+1] = 0

	csum := pkt.calcIPPseudoHeaderChecksum()

	// to handle odd lengths, we loop to length - 1, incrementing by 2, then
	// handle the last byte specifically by checking against the original
	// length.
	length := len(pkt.Payload) - 1
	for i := 0; i < length; i += 2 {
		// For our test packet, doing this manually is about 25% faster
		// (740 ns vs. 1000ns) than doing it by calling binary.BigEndian.Uint16.
		csum += uint32(pkt.Payload[i]) << 8
		csum += uint32(pkt.Payload[i+1])
	}
	if len(pkt.Payload)%2 == 1 {
		csum += uint32(pkt.Payload[length]) << 8
	}
	for csum > 0xffff {
		csum = (csum >> 16) + (csum & 0xffff)
	}
	networkByteOrder.PutUint16(pkt.Payload[csumIdx:], ^uint16(csum))
}

func (pkt *IPPacket) calcIPPseudoHeaderChecksum() (csum uint32) {
	csum += (uint32(pkt.Header[12]) + uint32(pkt.Header[14])) << 8
	csum += uint32(pkt.Header[13]) + uint32(pkt.Header[15])
	csum += (uint32(pkt.Header[16]) + uint32(pkt.Header[18])) << 8
	csum += uint32(pkt.Header[17]) + uint32(pkt.Header[19])

	length := uint32(len(pkt.Payload))
	csum += uint32(pkt.IPProto)
	csum += length & 0xffff
	csum += length >> 16

	return csum
}

func (pkt *IPPacket) FT() FiveTuple {
	return FiveTuple{
		IPProto: pkt.IPProto,
		Src:     Addr{IPString: pkt.SrcAddr.String(), Port: networkByteOrder.Uint16(pkt.Payload[0:2])},
		Dst:     Addr{IPString: pkt.DstAddr.String(), Port: networkByteOrder.Uint16(pkt.Payload[2:4])},
	}
}

type FiveTuple struct {
	IPProto uint8
	Src     Addr
	Dst     Addr
}

func (ft FiveTuple) Reversed() FiveTuple {
	return FiveTuple{
		IPProto: ft.IPProto,
		Src:     ft.Dst,
		Dst:     ft.Src,
	}
}

func (ft FiveTuple) String() string {
	return fmt.Sprintf("[%d] %v -> %v", ft.IPProto, ft.Src, ft.Dst)
}

type Addr struct {
	IPString string
	Port     uint16
}

func (a Addr) String() string {
	return fmt.Sprintf("%v:%d", a.IPString, a.Port)
}

func (a Addr) IP() net.IP {
	return net.ParseIP(a.IPString).To4()
}
