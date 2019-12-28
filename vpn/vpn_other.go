// +build !linux

package vpn

import (
	"github.com/getlantern/errors"
)

// Enable enables the VPN and returns a function that can be used to close the VPN when finished
func Enable(socksAddr, internetGateway, tunDeviceName, tunAddr, tunMask string) (func() error, error) {
	return nil, errors.New("VPN mode is currently only supported on linux")
}
