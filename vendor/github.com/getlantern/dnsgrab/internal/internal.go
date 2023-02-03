package internal

import (
	"encoding/binary"
	"net"
)

var (
	Endianness = binary.BigEndian

	// We use Class-E network space for fake IPs, which gives us the ability to
	// have up to 268435454 addresses in-flight (much more than we can
	// realistically cache anyway). Class-E is reserved for research, so there
	// aren't any real Internet services listening on any of these addresses.
	MinIP = IPStringToInt("240.0.0.1") // begin of Class-E network

	// MaxIP is the end of Class-E network
	MaxIP = IPStringToInt("255.255.255.254")
)

func IPStringToInt(ip string) uint32 {
	return IPToInt(net.ParseIP(ip))
}

func IPToInt(ip net.IP) uint32 {
	return Endianness.Uint32(ip.To4())
}

func IntToIP(i uint32) net.IP {
	ip := make(net.IP, net.IPv4len)
	Endianness.PutUint32(ip, i)
	return ip
}
