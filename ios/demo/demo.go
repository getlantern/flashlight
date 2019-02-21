// This demo program allows testing the iOS packet forwarding functionality
// on a desktop machine.
//
// Run the program, then in a terminal
//
// sudo route delete default
// sudo route add default 10.0.0.2
// sudo route add 67.205.172.79 192.168.1.1
//
// Replace 192.168.1.1 with the IP of your default gateway
//
// Now your network traffic will route through here to the proxy at 67.205.172.79.
//
// When you're finished, you can fix your routing table with:
//
// sudo route delete default
// sudo route delete 67.205.172.79
// sudo route add default 102.168.1.1
//
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/gotun"

	"github.com/getlantern/flashlight/ios"
)

var (
	log = golog.LoggerFor("ios-demo")
)

var (
	tunDevice = flag.String("tun-device", "tun0", "tun device name")
	tunAddr   = flag.String("tun-address", "10.0.0.2", "tun device address")
	tunMask   = flag.String("tun-mask", "255.255.255.0", "tun device netmask")
	tunGW     = flag.String("tun-gw", "10.0.0.1", "tun device gateway")
	ifOut     = flag.String("ifout", "en0", "name of interface to use for outbound connections")
)

type fivetuple struct {
	proto            string
	srcIP, dstIP     string
	srcPort, dstPort int
}

func (ft fivetuple) String() string {
	return fmt.Sprintf("[%v] %v:%v -> %v:%v", ft.proto, ft.srcIP, ft.srcPort, ft.dstIP, ft.dstPort)
}

func main() {
	dev, err := tun.OpenTunDevice(*tunDevice, *tunAddr, *tunGW, *tunMask)
	if err != nil {
		log.Fatal(err)
	}
	defer dev.Stop()

	outIF, err := net.InterfaceByName(*ifOut)
	if err != nil {
		log.Fatal(err)
	}
	outIFAddrs, err := outIF.Addrs()
	if err != nil {
		log.Fatal(err)
	}
	var laddrTCP *net.TCPAddr
	var laddrUDP *net.UDPAddr
	for _, outIFAddr := range outIFAddrs {
		switch t := outIFAddr.(type) {
		case *net.IPNet:
			ipv4 := t.IP.To4()
			if ipv4 != nil {
				laddrTCP = &net.TCPAddr{IP: ipv4, Port: 0}
				laddrUDP = &net.UDPAddr{IP: ipv4, Port: 0}
				break
			}
		}
	}
	if laddrTCP == nil {
		log.Fatalf("Unable to get IPv4 address for interface %v", *ifOut)
	}
	log.Debugf("Outbound TCP will use %v", laddrTCP)
	log.Debugf("Outbound UDP will use %v", laddrUDP)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		<-ch
		log.Debug("Stopping TUN device")
		dev.Stop()
		log.Debug("Stopped TUN device")
	}()

	err = ios.StartWithDevice(dev)
	if err != nil {
		log.Fatal(err)
	}
	time.Sleep(1000 * time.Hour)
}
