package gonat

import (
	"io"
	"os/exec"
	"strconv"

	"github.com/getlantern/errors"
	"github.com/songgao/water"
)

// TUNDevice creates a TUN device with the given name and configures an interface for that TUN
// device at the given address and netmask and given mtu (should usually be 1500).
func TUNDevice(name, addr, netmask string, mtu int) (io.ReadWriteCloser, error) {
	cfg := water.Config{DeviceType: water.TUN}
	cfg.Name = name
	dev, err := water.New(cfg)
	if err != nil {
		return nil, errors.New("error opening TUN device: %v", err)
	}
	log.Debugf("Created TUN device named %v", dev.Name())

	if out, configErr := exec.Command("ifconfig", dev.Name(), addr, "netmask", netmask, "mtu", strconv.Itoa(mtu)).CombinedOutput(); configErr != nil {
		dev.Close()
		return nil, errors.New("failed to configure tun device address: %v", string(out))
	}

	return dev, nil
}
