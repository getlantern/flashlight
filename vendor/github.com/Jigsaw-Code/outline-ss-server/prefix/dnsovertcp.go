package prefix

import (
	cryptoRand "crypto/rand"
	"fmt"
)

const dnsOverTCPMsgLen = 1500

type dnsOverTCPPrefix struct{}

func NewDNSOverTCPPrefix() Prefix {
	return dnsOverTCPPrefix{}
}

// See here for more info about this number:
// https://github.com/getlantern/lantern-internal/issues/4428#issuecomment-1337979698
//
// In short, it's a length that worked for Shadowsocks on Iran's MCI ISP. It's
// just easier to hardcode it until we need to make it variable
func (p dnsOverTCPPrefix) Make() ([]byte, error) {
	if dnsOverTCPMsgLen >= 0xffff {
		return nil, fmt.Errorf("Invalid length %d", dnsOverTCPMsgLen)
	}

	b := make([]byte, 2)
	_, err := cryptoRand.Read(b)
	if err != nil {
		return nil, fmt.Errorf("Unable to generate random bytes for DNS-over-TCP prefix: %w", err)
	}
	len := dnsOverTCPMsgLen
	prefix := []byte{
		byte(len >> 8), byte(len), // Length
		b[0], b[1], // Transaction ID
		0x01, 0x20, // Flags: Standard query, recursion desired
	}
	return prefix, nil
}
