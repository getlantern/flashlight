package chained

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/getlantern/bandwidth"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/ops"
)

// Config is a configuration for a Dialer.
type Config struct {
	// DialServer: function that dials the upstream server proxy
	DialServer func() (net.Conn, error)

	// OnRequest: optional function that gets called on every CONNECT request to
	// the server and is allowed to modify the http.Request before it passes to
	// the server.
	OnRequest func(req *http.Request)

	// OnFinish: optional function that gets called when finishe the tracking of
	// xfer operations, allows adding additional data to op context.
	OnFinish func(op *ops.Op)

	OnConnectResponse func(requestTime time.Time, resp *http.Response)

	// ShouldResetBBR indicates whether the server should be told to reset BBR
	// metrics.
	ShouldResetBBR func() bool

	// Label: a optional label for debugging.
	Label string
}

// dialer provides an implementation of net.Dial that proxies traffic via an
// upstream server proxy. Its Dial function uses DialServer to dial the server
// proxy and then issues a CONNECT request to instruct the server to connect to
// the destination at the specified network and addr.
type dialer struct {
	Config
}

// NewDialer returns an implementation of net.Dial() based on the given Config.
func NewDialer(cfg Config) func(network, addr string) (net.Conn, error) {
	d := &dialer{Config: cfg}
	return d.Dial
}

// Dial is a net.Dial-compatible function.
func (d *dialer) Dial(network, addr string) (net.Conn, error) {
	conn, err := d.DialServer()
	if err != nil {
		return nil, errors.New("Unable to dial server %v: %s", d.Label, err)
	}
	// Look for our special hacked "connect" transport used to signal
	// that we should send a CONNECT request and tunnel all traffic through
	// that.
	switch network {
	case "connect":
		log.Tracef("Sending CONNECT request")
		err = d.sendCONNECT(addr, conn)
	case "persistent":
		log.Tracef("Sending GET request to establish persistent HTTP connection")
		err = d.initPersistentConnection(addr, conn)
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	return withRateTracking(conn, addr, d.OnFinish), nil
}

func (d *dialer) sendCONNECT(addr string, conn net.Conn) error {
	reqTime := time.Now()
	req, err := d.buildCONNECTRequest(addr, d.OnRequest)
	if err != nil {
		return fmt.Errorf("Unable to construct CONNECT request: %s", err)
	}
	err = req.Write(conn)
	if err != nil {
		return fmt.Errorf("Unable to write CONNECT request: %s", err)
	}

	r := bufio.NewReader(conn)
	err = d.checkCONNECTResponse(r, req, reqTime)
	return err
}

func (d *dialer) buildCONNECTRequest(addr string, onRequest func(req *http.Request)) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodConnect, "/", nil)
	if err != nil {
		return nil, err
	}
	req.URL = &url.URL{
		Host: addr,
	}
	req.Host = addr
	if onRequest != nil {
		onRequest(req)
	}
	// Request BBR metrics
	bbrOption := "y"
	if d.ShouldResetBBR() {
		bbrOption = "clear"
	}
	req.Header.Set("X-BBR", bbrOption)
	return req, nil
}

func (d *dialer) checkCONNECTResponse(r *bufio.Reader, req *http.Request, reqTime time.Time) error {
	resp, err := http.ReadResponse(r, req)
	if err != nil {
		return fmt.Errorf("Error reading CONNECT response: %s", err)
	}
	if !sameStatusCodeClass(http.StatusOK, resp.StatusCode) {
		var body []byte
		if resp.Body != nil {
			defer resp.Body.Close()
			body, _ = ioutil.ReadAll(resp.Body)
		}
		log.Errorf("Bad status code on CONNECT response %d: %v", resp.StatusCode, string(body))
		return balancer.ErrUpstream
	}
	d.OnConnectResponse(reqTime, resp)
	bandwidth.Track(resp)
	return nil
}

func sameStatusCodeClass(statusCode1 int, statusCode2 int) bool {
	// HTTP response status code "classes" come in ranges of 100.
	const classRange = 100
	// These are all integers, so division truncates.
	return statusCode1/classRange == statusCode2/classRange
}

func (d *dialer) initPersistentConnection(addr string, conn net.Conn) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%v", addr), nil)
	if err != nil {
		return err
	}
	if d.OnRequest != nil {
		d.OnRequest(req)
	}
	req.Header.Set("X-Lantern-Persistent", "true")
	writeErr := req.Write(conn)
	if writeErr != nil {
		return fmt.Errorf("Unable to write initial request: %v", writeErr)
	}

	return nil
}
