// Package hellocap is used to capture sample TLS ClientHellos from web browsers on this machine.
package hellocap

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"sync"

	"github.com/getlantern/tlsutil"
)

type browser interface {
	// Uses the browser to make an HTTP GET request of the input address.
	//
	// This function must return an error if and only if the GET was not sent. Any errors after the
	// request has been sent must be ignored. In cases where it is unclear whether the GET was sent
	// successfully, it is better to assume that it was. The caller can always time out if they
	// never see the request.
	get(ctx context.Context, addr string) error

	// The browser's name, e.g. Google Chrome.
	name() string

	// Free up resources used by this browser instance.
	close() error
}

// GetDefaultBrowserHello returns a sample ClientHello from the system's default web browser. This
// function may take a couple of seconds.
//
// Note that this may generate a log from the http package along the lines of "http: TLS handshake
// error... use of closed network connection".
func GetDefaultBrowserHello(ctx context.Context) ([]byte, error) {
	b, err := defaultBrowser(ctx)
	if err != nil {
		return nil, err
	}
	defer b.close()
	return getBrowserHello(ctx, b)
}

func getBrowserHello(ctx context.Context, browser browser) ([]byte, error) {
	type helloResult struct {
		hello []byte
		err   error
	}

	var (
		helloChan      = make(chan helloResult, 1)
		serverErrChan  = make(chan error, 1)
		browserErrChan = make(chan error, 1)
		sendHelloOnce  = sync.Once{}
	)

	s, err := newCapturingServer(func(hello []byte, err error) {
		sendHelloOnce.Do(func() { helloChan <- helloResult{hello, err} })
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start hello-capture server: %w", err)
	}
	defer s.Close()

	go func() {
		if err := s.listenAndServeTLS(); err != nil {
			serverErrChan <- err
		}
	}()
	go func() {
		// Grab the port and use localhost to ensure the browser sends an SNI.
		_, port, err := net.SplitHostPort(s.address())
		if err != nil {
			browserErrChan <- fmt.Errorf("failed to parse test server address: %w", err)
			return
		}
		if err := browser.get(ctx, fmt.Sprintf("https://localhost:%s", port)); err != nil {
			browserErrChan <- err
		}
	}()

	select {
	case result := <-helloChan:
		return result.hello, result.err
	case err := <-serverErrChan:
		return nil, fmt.Errorf("error serving browser connection: %w", err)
	case err := <-browserErrChan:
		return nil, fmt.Errorf("failed to make test request with %s: %w", browser.name(), err)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// onHello is a callback invoked when a ClientHello is captured. Only one of hello or err will be
// non-nil. This callback is not invoked for incomplete ClientHellos. In other words, err is non-nil
// only if what has been read off the connection could not possibly constitute a ClientHello,
// regardless of further data from the connection.
type onHello func(hello []byte, err error)

type capturingConn struct {
	// Wraps a TCP connection.
	net.Conn

	helloLock sync.Mutex
	helloRead bool
	helloBuf  bytes.Buffer
	onHello   onHello
}

func (c *capturingConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	c.checkHello(b[:n])
	return
}

func (c *capturingConn) Write(b []byte) (n int, err error) {
	return c.Conn.Write(b)
}

func (c *capturingConn) checkHello(newBytes []byte) {
	c.helloLock.Lock()
	if !c.helloRead {
		c.helloBuf.Write(newBytes)
		nHello, parseErr := tlsutil.ValidateClientHello(c.helloBuf.Bytes())
		if parseErr == nil {
			c.onHello(c.helloBuf.Bytes()[:nHello], nil)
			c.helloRead = true
		} else if !errors.Is(parseErr, io.EOF) {
			c.onHello(nil, fmt.Errorf("could not parse captured bytes as a ClientHello: %w", parseErr))
			c.helloRead = true
		}
	}
	c.helloLock.Unlock()
}

type capturingListener struct {
	net.Listener
	onHello onHello
}

func (l capturingListener) Accept() (net.Conn, error) {
	tcpConn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &capturingConn{Conn: tcpConn, onHello: l.onHello}, nil
}

func listenAndCaptureTCP(onHello onHello) (*capturingListener, error) {
	l, err := net.Listen("tcp", "")
	if err != nil {
		return nil, err
	}
	return &capturingListener{l, onHello}, nil
}

type capturingServer struct {
	s *http.Server
	l capturingListener
}

func newCapturingServer(onHello onHello) (*capturingServer, error) {
	l, err := listenAndCaptureTCP(onHello)
	if err != nil {
		return nil, fmt.Errorf("failed to start TLS server for capturing hellos: %w", err)
	}
	s := http.Server{
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	return &capturingServer{&s, *l}, nil
}

func (cs *capturingServer) listenAndServeTLS() error {
	return cs.s.ServeTLS(cs.l, "", "")
}

func (cs *capturingServer) address() string {
	return cs.l.Addr().String()
}

func (cs *capturingServer) Close() error {
	return cs.s.Close()
}

func wrapExecError(msg string, err error) error {
	var execErr *exec.ExitError
	if errors.As(err, &execErr) {
		return fmt.Errorf("%w: %s", err, string(execErr.Stderr))
	}
	return err
}

var (
	certPem = []byte(`-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----`)
	keyPem = []byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIIrYSSNQFaA2Hwf1duRSxKtLYX5CB04fSeQ6tF1aY/PuoAoGCCqGSM49
AwEHoUQDQgAEPR3tU2Fta9ktY+6P9G0cWO+0kETA6SFs38GecTyudlHz6xvCdz8q
EKTcWGekdmdDPsHloRNtsiCa697B2O9IFA==
-----END EC PRIVATE KEY-----`)

	cert tls.Certificate
)

func init() {
	var err error
	cert, err = tls.X509KeyPair(certPem, keyPem)
	if err != nil {
		panic(err)
	}
}
