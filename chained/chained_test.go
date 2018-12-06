package chained

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/golog"
)

var (
	logger = golog.LoggerFor("chained-test")
	ping   = []byte("ping")
	pong   = []byte("pong")
)

func newTestUserConfig() *common.UserConfigData {
	return common.NewUserConfigData("device", 1234, "protoken", nil, "en-US")
}

func NewDialer(dialServer func(ctx context.Context, p *proxy) (net.Conn, error)) (func(network, addr string) (net.Conn, error), error) {
	p, err := newProxy("test", "proto", "netw", &ChainedServerInfo{
		Addr:      "addr:567",
		AuthToken: "token",
	}, newTestUserConfig(), true, false, dialServer, defaultDialOrigin)
	if err != nil {
		return nil, err
	}
	return p.dial, nil
}

func TestBadDialServer(t *testing.T) {
	dialer, err := NewDialer(func(ctx context.Context, p *proxy) (net.Conn, error) {
		return nil, fmt.Errorf("I refuse to dial")
	})
	if !assert.NoError(t, err) {
		return
	}
	_, err = dialer("tcp", "www.google.com")
	assert.Error(t, err, "Dialing with a bad DialServer function should have failed")
}

func TestBadProtocol(t *testing.T) {
	dialer, err := NewDialer(func(ctx context.Context, p *proxy) (net.Conn, error) {
		return net.Dial("tcp", "www.google.com")
	})
	if !assert.NoError(t, err) {
		return
	}
	_, err = dialer("udp", "www.google.com")
	assert.Error(t, err, "Dialing with a non-tcp protocol should have failed")
}

func TestBadServer(t *testing.T) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Unable to listen: %s", err)
	}

	go func() {
		conn, err := l.Accept()
		if err == nil {
			if err := conn.Close(); err != nil {
				t.Fatalf("Unable to close connection: %v", err)
			}
		}
	}()

	dialer, err := NewDialer(func(ctx context.Context, p *proxy) (net.Conn, error) {
		return net.Dial("tcp", l.Addr().String())
	})
	if !assert.NoError(t, err) {
		return
	}
	_, err = dialer("connect", "www.google.com")
	logger.Debugf("Error: %v", err)
	assert.Error(t, err, "Dialing a server that disconnects too soon should have failed")
}

func TestBadConnectStatus(t *testing.T) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Unable to listen: %s", err)
	}

	hs := &http.Server{
		Handler: http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			resp.WriteHeader(403) // forbidden
		}),
	}
	go func() {
		if err := hs.Serve(l); err != nil {
			t.Fatalf("Unable to serve: %v", err)
		}
	}()

	dialer, err := NewDialer(func(ctx context.Context, p *proxy) (net.Conn, error) {
		return net.DialTimeout("tcp", l.Addr().String(), 2*time.Second)
	})
	if !assert.NoError(t, err) {
		return
	}
	_, err = dialer("connect", "www.google.com")
	assert.Error(t, err, "Dialing a server that sends a non-successful HTTP status to our CONNECT request should have failed")
}

func TestBadMethodToServer(t *testing.T) {
	l := startServer(t)
	resp, err := http.Get("http://" + l.Addr().String() + "/")
	assert.NoError(t, err, "Making a Get request to the server should not have errored")
	if err == nil {
		assert.True(t, resp.StatusCode == 405, "Response should have indicated a bad method")
	}
}

func TestBadAddressToServer(t *testing.T) {
	p, err := newProxy("test", "proto", "netw", &ChainedServerInfo{
		Addr:      "addr:567",
		AuthToken: "token",
	}, newTestUserConfig(), true, false, func(ctx context.Context, p *proxy) (net.Conn, error) {
		return nil, fmt.Errorf("fail intentionally")
	}, defaultDialOrigin)
	if !assert.NoError(t, err) {
		return
	}
	l := startServer(t)
	req, err := p.buildCONNECTRequest("somebadaddressasdfdasfds.asdfasdf.dfads:532400")
	if err != nil {
		t.Fatalf("Unable to build request: %s", err)
	}
	conn, err := net.DialTimeout("tcp", l.Addr().String(), 10*time.Second)
	if err != nil {
		t.Fatalf("Unable to dial server: %s", err)
	}
	err = req.Write(conn)
	if err != nil {
		t.Fatalf("Unable to make request: %s", err)
	}

	r := bufio.NewReader(conn)
	op := ops.Begin("test_op")
	defer op.End()
	err = p.checkCONNECTResponse(op, r, req, time.Now())
	assert.Error(t, err, "Connect response should be bad")
}

func TestSuccess(t *testing.T) {
	l := startServer(t)

	dialer, err := NewDialer(func(ctx context.Context, p *proxy) (net.Conn, error) {
		logger.Debugf("Dialing with timeout to: %v", l.Addr())
		conn, err := net.DialTimeout(l.Addr().Network(), l.Addr().String(), 2*time.Second)
		logger.Debugf("Got conn %v and err %v", conn, err)
		return conn, err
	})
	if !assert.NoError(t, err) {
		return
	}

	logger.Debugf("TESTING SUCCESS")
	test(t, dialer)
}

func startServer(t *testing.T) net.Listener {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Unable to listen: %s", err)
	}

	s := &Server{
		Dial: net.Dial,
	}
	go func() {
		err := s.Serve(l)
		if err != nil {
			t.Fatalf("Unable to serve: %s", err)
		}
	}()

	return l
}

// test tests a Dialer.
func test(t *testing.T, dialer func(network, addr string) (net.Conn, error)) {
	// Set up listener for server endpoint
	sl, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Unable to listen: %s", err)
	}

	// Server that responds to ping
	go func() {
		conn, err := sl.Accept()
		if err != nil {
			t.Fatalf("Unable to accept connection: %s", err)
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Logf("Unable to close connection: %v", err)
			}
		}()
		b := make([]byte, 4)
		_, err = io.ReadFull(conn, b)
		if err != nil {
			t.Fatalf("Unable to read from client: %s", err)
		}
		assert.Equal(t, ping, b, "Didn't receive correct ping message")
		_, err = conn.Write(pong)
		if err != nil {
			t.Fatalf("Unable to write to client: %s", err)
		}
	}()

	conn, err := dialer("connect", sl.Addr().String())
	if err != nil {
		t.Fatalf("Unable to dial via proxy: %s", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("Unable to close connection: %v", err)
		}
	}()

	_, err = conn.Write(ping)
	if err != nil {
		t.Fatalf("Unable to write to server via proxy: %s", err)
	}

	b := make([]byte, 4)
	_, err = io.ReadFull(conn, b)
	if err != nil {
		t.Fatalf("Unable to read from server: %s", err)
	}
	assert.Equal(t, pong, b, "Didn't receive correct pong message")
}

func (p *proxy) dial(network, addr string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), chainedDialTimeout)
	defer cancel()
	conn, _, err := p.DialContext(ctx, network, addr)
	return conn, err
}

func TestCiphersFromNames(t *testing.T) {
	assert.Nil(t, ciphersFromNames(nil))
	assert.Nil(t, ciphersFromNames([]string{}))
	assert.Nil(t, ciphersFromNames([]string{"UNKNOWN"}))
	assert.EqualValues(t, []uint16{0x0035, 0x003c}, ciphersFromNames([]string{"TLS_RSA_WITH_AES_256_CBC_SHA", "UNKNOWN", "TLS_RSA_WITH_AES_128_CBC_SHA256"}))
}
