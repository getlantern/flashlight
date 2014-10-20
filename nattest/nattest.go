// package nattest provides the capability to test a nat-traversed UDP
// connection by sending packets back and forth between client and server.
package nattest

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/nattywad"
)

const (
	NumUDPTestPackets = 10
	ConnTimeout       = NumUDPTestPackets * time.Second
)

var (
	log = golog.LoggerFor("flashlight.nattest")
)

type Record func(*net.UDPConn, bool)

func Ping(local *net.UDPAddr, remote *net.UDPAddr, record Record) {
	conn, err := net.DialUDP("udp", local, remote)
	conn.SetDeadline(time.Now().Add(ConnTimeout))

	if err != nil {
		log.Errorf("nattest unable to dial UDP: %s", err)
		return
	}

	go func() {
		for i := 0; i < NumUDPTestPackets; i++ {
			msg := fmt.Sprintf("Hello from %s to %s", local, remote)
			log.Tracef("nattest sending UDP message: %s", msg)
			_, err := conn.Write([]byte(msg))
			if err != nil {
				log.Debugf("nattest unable to write to UDP: %v", err)
				return
			}
			time.Sleep(time.Second)
		}
	}()

	go func() {
		b := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(ConnTimeout))
		for {
			n, addr, err := conn.ReadFrom(b)
			if err != nil {
				// io.EOF should indicate that the connection
				// is closed by the other end
				if err == io.EOF {
					record(conn, false)
					return
				} else {
					log.Errorf("nattest error reading UDP packet %v", err)
					time.Sleep(time.Second)
					continue
				}
			}
			log.Debugf("nattest received echo from %v %d", addr, n)
			record(conn, true)
			return
		}
	}()
}

func ConfirmConnectivity(conn *net.UDPAddr) {

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
			_, err = conn.Write([]byte(nattywad.ServerReady))
			if err != nil {
				log.Debugf("nattest unable to write to UDP: %v", err)
				return
			}

		}
	}()
	log.Debugf("nattest listening for UDP packets at: %s", local)
	return nil
}
