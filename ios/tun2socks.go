package flashlightios

import (
	"github.com/getlantern/errors"
	"github.com/getlantern/gotun2socks"
	"github.com/getlantern/gotun2socks/tun"
)

func Tun2Socks(fd int, socksAddr string, dnsServer string) error {
	dev, err := tun.WrapTunDevice(fd)
	if err != nil {
		return errors.New("Unable to wrap TUN device: %v", err)
	}
 	tun := gotun2socks.New(dev,
		socksAddr,
		[]string{dnsServer},
		true,  // public traffic only
		false, // don't cache DNS
	)
	go tun.Run()
 	log.Debug("Listening for TUN traffic")
	return nil
}