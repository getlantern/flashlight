// Package hellocap is used to capture example TLS ClientHellos from the user's default browser.
//
// TODO: add more context
// TODO: should this be a subdirectory of chained or some other package?
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
	"time"

	"github.com/getlantern/tlsutil"
)

// A DomainMapper is used to ensure that captured ClientHellos are accurate for a specific domain.
// Because browsers may send one type of hello to one domain and another type of hello to others, it
// is important to capture the correct ClientHello for the intended domain.
//
// When MapTo is called, all connections to the domain should go to the specified address. This is
// analogous to editing the system's /etc/hosts file. Clear should undo this.
type DomainMapper interface {
	Domain() string
	MapTo(address string) error
	Clear() error
}

// GetBrowserHello returns a sample ClientHello from the system's default web browser.
func GetBrowserHello(ctx context.Context, dm DomainMapper) ([]byte, error) {
	b, err := defaultBrowser(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to obtain user's default browser: %w", err)
	}
	defer b.close()
	return getBrowserHello(ctx, b, dm)
}

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

type chrome struct {
	path string
}

func (c chrome) name() string { return "Google Chrome" }
func (c chrome) close() error { return nil }

func (c chrome) get(ctx context.Context, addr string) error {
	// The --disable-gpu flag is necessary to run headless Chrome on Windows:
	// https://bugs.chromium.org/p/chromium/issues/detail?id=737678
	if err := exec.CommandContext(ctx, c.path, "--headless", "--disable-gpu", addr).Run(); err != nil {
		// The Chrome binary does not appear to ever exit non-zero, so we don't need to worry about
		// catching and ignoring errors due to things like certificate validity checks.
		return fmt.Errorf("failed to execute binary: %w", err)
	}
	return nil
}

func getBrowserHello(ctx context.Context, browser browser, dm DomainMapper) ([]byte, error) {
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

	sHost, sPort, err := net.SplitHostPort(s.address())
	if err != nil {
		return nil, fmt.Errorf("failed to parse local server address: %w", err)
	}

	if err := dm.MapTo(sHost); err != nil {
		return nil, fmt.Errorf("failed to map domain: %w", err)
	}
	defer dm.Clear()

	go func() {
		if err := s.listenAndServeTLS(); err != nil {
			serverErrChan <- err
		}
	}()
	go func() {
		// TODO: think about whether replacing the port here will work w.r.t. the proxy dialer
		if err := browser.get(ctx, fmt.Sprintf("https://%s:%s", dm.Domain(), sPort)); err != nil {
			browserErrChan <- err
		}
	}()

	// debugging
	// TODO: delete me
	defer func() { fmt.Printf("[%v] returning hello\n", time.Now()) }()

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

// TODO: port tests from helloreader
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
	// TODO: explain why we use IPv4... or do we need to?
	l, err := net.Listen("tcp4", "127.0.0.1:")
	if err != nil {
		return nil, err
	}
	return &capturingListener{l, onHello}, nil
}

// StartServer is for debugging.
// TODO: delete
func StartServer(onHello func(hello []byte, err error), serverErrors chan<- error) (addr string, close func() error, err error) {
	s, err := newCapturingServer(onHello)
	if err != nil {
		return "", nil, err
	}
	go func() { serverErrors <- s.listenAndServeTLS() }()
	return s.l.Addr().String(), s.Close, nil
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
