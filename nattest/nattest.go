// package nattest provides the capability to test a nat-traversed UDP
// connection by sending packets back and forth between client and server.
package nattest

import (
	"fmt"
	"net"
	"time"

	"github.com/getlantern/golog"
)

const (
	NumUDPTestPackets = 10
)

var (
	log = golog.LoggerFor("flashlight.nattest")
)

func Ping(local *net.UDPAddr, remote *net.UDPAddr) {
	conn, err := net.DialUDP("udp", local, remote)
	if err != nil {
		log.Errorf("nattest unable to dial UDP: %s", err)
		return
	}
	for i := 0; i < NumUDPTestPackets; i++ {
		msg := fmt.Sprintf("Hello from %s to %s", local, remote)
		log.Debugf("nattest sending UDP message: %s", msg)
		_, err := conn.Write([]byte(msg))
		if err != nil {
			log.Errorf("nattest unable to write to UDP: %s", err)
			return
		}
		time.Sleep(1 * time.Second)
	}
	conn.Close()
}

func Serve(local *net.UDPAddr) error {
	conn, err := net.ListenUDP("udp", local)
	if err != nil {
		return fmt.Errorf("Unable to listen on UDP: %s", err)
	}

	go func() {
		startTime := time.Now()
		b := make([]byte, 1024)
		for {
			if time.Now().Sub(startTime) > 30*time.Second {
				log.Debugf("nattest stopped listening for UDP packets at: %s", local)
				return
			}
			n, addr, err := conn.ReadFrom(b)
			if err != nil {
				log.Fatalf("Unable to read from UDP: %s", err)
			}
			msg := string(b[:n])
			log.Debugf("Got UDP message from %s: '%s'", addr, msg)
		}
	}()
	log.Debugf("nattest listening for UDP packets at: %s", local)
	return nil
}
