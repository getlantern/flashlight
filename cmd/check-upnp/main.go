// Example run:
//
// 		go run cmd/check-upnp/main.go --p2p-registrar-endpoint=https://replica-p2p-registrar.fly.dev
//
//
// This binary does the following:
//
// - Takes a [replica-p2p-registrar](https://github.com/getlantern/replica-p2p-registrar) URL as a CLI argument.
// - Runs a UDP server on a random port that just echos back whatever is sent to it
// - Attempts to portforward the UDP port with our "flashlight/upnp" package
// - Sends a GET request to "/echo-udp-msg" route of the registrar service with
//   - "addr" as a URL parameter, which'll be the ip:port of the UDP server
//   - "msg" as a URL parameter, which'll be the message to echo
// - Upon receiving the /echo-udp-msg request, the replica-p2p-registrar service:
//   - Sends a UDP packet containing "msg" to the UDP server running on "addr"
//   - The UDP server running on "addr" will have to echo back the received message
//   - replica-p2p-registrar receives that echoed-back message and compares it
//     with the sent message. They must be identical
// - If the above works just fine, replica-p2p-registrar responds to the
//   /echo-udp-msg with a 200 OK, confirming that upnp worked and the port is
//   open
//
// It goes like this
//
//                         GET /echo-udp-msg?msg=bunnyfoofoo
//     +-----------------+  --------------------------->    +---------------------------+
//     |                 |                                  |                           |
//     |                 |      UDP packet: bunnyfoofoo     |                           |
//     |  cmd/check-upnp |  <---------------------------    |   replica-p2p-registrar   |
//     |                 |                                  |                           |
//     +-----------------+      UDP packet: bunnyfoofoo     +---------------------------+
//                          --------------------------->
//
//                              Confirmed port is open.
//                              Respond to /echo-udp-msg
//                              with 200 OK
//                          <---------------------------
package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/anacrolix/publicip"
	"github.com/getlantern/flashlight/upnp"
	"github.com/getlantern/golog"
	"golang.org/x/sync/errgroup"
)

var (
	p2pRegistrarEndpointFlag = flag.String("p2p-registrar-endpoint", "", "")

	log       = golog.LoggerFor("cmd-check-upnp")
	msgtoSend = "bunnyfoofoo"
)

func main() {
	if err := mainErr(); err != nil {
		log.Fatalf(err.Error())
	}
}

func mainErr() error {
	flag.Parse()
	if *p2pRegistrarEndpointFlag == "" {
		return log.Errorf("p2p-registrar-endpoint is nil")
	}
	log.Debugf("p2p-registrar-endpoint is %v", *p2pRegistrarEndpointFlag)

	// Start a UDP server
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return log.Errorf(" %v", err)
	}
	defer conn.Close()

	// Fetch public ip
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pubip, err := publicip.Get4(ctx)
	if err != nil {
		return log.Errorf("failed to fetch public ip %v", err)
	}

	port := conn.LocalAddr().(*net.UDPAddr).Port
	errChan := make(chan error)
	go func() {
		// Listen for a single UDP msg
		buf := make([]byte, 256)
		n, dstAddr, err := conn.ReadFrom(buf)
		if err != nil {
			errChan <- log.Errorf(" %v", err)
		}
		// Write a UDP msg back
		_, err = conn.WriteTo(buf[:n], dstAddr)
		if err != nil {
			errChan <- log.Errorf(" %v", err)
		}
		close(errChan)
	}()

	// Attempt to portforward the port
	err = upnp.New().ForwardPortWithUpnp(uint16(port), "udp")
	if err != nil {
		return log.Errorf("Failed to portforward [udp%d]: %v", port, err)
	}

	// Do two things and die if one of them fails:
	// - Monitor all read/write errors from the UDP server
	// - Run a GET request to our registrar service asking it to echo a UDP msg
	//   we send it on our open port. If that was successful, this port is open
	//   and Upnp works
	g := new(errgroup.Group)
	g.Go(func() error {
		err := <-errChan
		if err != nil {
			return err
		}
		return nil
	})
	g.Go(func() error {
		req, err := http.NewRequest(
			"GET",
			*p2pRegistrarEndpointFlag+"/echo-udp-msg",
			nil)
		if err != nil {
			return log.Errorf("while making registar-endpoint request %v", err)
		}
		q := req.URL.Query()
		q.Add("addr", fmt.Sprintf("%s:%d", pubip.To4().String(), port))
		q.Add("msg", msgtoSend)
		req.URL.RawQuery = q.Encode()
		cl := &http.Client{Timeout: 10 * time.Second}
		resp, err := cl.Do(req)
		if err != nil {
			return log.Errorf("while http.Client.Do() %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return log.Errorf("failed to read failed request's body %v", err)
			}
			defer resp.Body.Close()
			return log.Errorf(
				"/echo-udp-msg route failed with [%v] and body [%v]",
				resp.StatusCode, string(b))
		}
		log.Debugf("Portforwarding for udp:%d was successful!", port)
		return nil
	})
	return g.Wait()
}
