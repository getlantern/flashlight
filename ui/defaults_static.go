// +build disableresourcerandomization

package ui

import "net"

var defaultUIAddresses = []*net.TCPAddr{&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 16823}}

const strictOriginCheck = false

// LocalHTTPToken is a no-op without resource randomization.
func LocalHTTPToken() string {
	// no-op.
	return ""
}
