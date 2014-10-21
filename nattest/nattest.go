// package nattest provides the capability to test a nat-traversed UDP
// connection by sending packets back and forth between client and server.
package nattest

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/getlantern/golog"
)

const (
	NumUDPTestPackets = 10
	PingPause         = 2 * time.Second
	ConnTimeout       = NumUDPTestPackets * PingPause
)

var (
	log = golog.LoggerFor("flashlight.nattest")

	pingMsg = []byte("ping")
	pongMsg = []byte("pong")
)

type Record func(*net.UDPConn, bool)

// Ping pings the server at the other end of a NAT-traversed UDP connection and
// looks for echo packets to confirm connectivity with the server. It returns
// true if connectivity was confirmed, false otherwise.
func Ping(local *net.UDPAddr, remote *net.UDPAddr) bool {
	conn, err := net.DialUDP("udp", local, remote)
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(ConnTimeout))

	if err != nil {
		log.Debugf("Unable to dial UDP: %s", err)
		return false
	}

	// Send ping packets
	go func() {
		for i := 0; i < NumUDPTestPackets; i++ {
			_, err := conn.Write(pingMsg)
			if err != nil {
				log.Tracef("Unable to write to UDP: %v", err)
				return
			}
			time.Sleep(PingPause)
		}
	}()

	// Read pong packets
	b := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(ConnTimeout))
	for i := 0; i < NumUDPTestPackets; i++ {
		n, addr, err := conn.ReadFrom(b)
		if err != nil {
			// io.EOF should indicate that the connection
			// is closed by the other end
			if err == io.EOF {
				return false
			} else {
				log.Debugf("Error reading UDP packet %v", err)
				time.Sleep(time.Second)
				continue
			}
		}
		msg := b[:n]
		if bytes.Equal(msg, pongMsg) {
			log.Tracef("Received pong from %v %d", addr, n)
			return true
		} else {
			log.Debugf("Client received unexpected message from %v: %s", addr, msg)
		}
		return true
	}

	return false
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
				log.Tracef("nattest stopped listening for UDP packets at: %s", local)
				return
			}
			n, addr, err := conn.ReadFrom(b)
			if err != nil {
				log.Debugf("Unable to read from UDP: %s", err)
			}
			msg := b[:n]
			log.Tracef("Got UDP message from %s: '%s'", addr, msg)
			if bytes.Equal(msg, pingMsg) {
				// We got a ping message, respond
				_, err = conn.WriteTo([]byte(pongMsg), addr)
				if err != nil {
					log.Debugf("nattest unable to write to UDP: %v", err)
					return
				}
			} else {
				log.Debugf("Server received unexpected message: %s", msg)
			}
		}
	}()
	log.Tracef("nattest listening for UDP packets at: %s", local)
	return nil
}
