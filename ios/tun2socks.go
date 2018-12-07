package flashlightios

import (
	"fmt"
	"github.com/getlantern/errors"
	"github.com/getlantern/gotun2socks"
	"github.com/getlantern/gotun2socks/tun"

	"time"
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

	ch := make(chan []byte, 0)
	go func() {
		for range ch {
			// do nothing
		}
	}()

	go func() {
		for {
			ch <- make([]byte, 81920)
			time.Sleep(10 * time.Millisecond)
		}
	}()

	fmt.Println("Listening for go traffic")
	return nil
}
