package quicproxy

import (
	"context"
	"crypto/tls"
	stderr "errors"
	"net"
	"net/http"
	"strings"

	"github.com/elazarl/goproxy"
	"github.com/lucas-clemente/quic-go"
)

var (
	ErrQuicListenerClosed error = stderr.New("ErrQuicListenerClosed")
	SupportedTLSProtos          = []string{"quic-proxy"}
)

// QuicReverseProxy is an HTTP CONNECT proxy that uses the QUIC protocol for
// dialing. Use it to communicate with a quicproxy.QuicForwardProxy.
//
// In other words, this proxy converts QUIC traffic to HTTP.
//
// The ingress traffic to this proxy QUIC
// The egress traffic of this proxy is regular HTTP through TCP.
//
// HTTP3 is not used here.
type QuicReverseProxy struct {
	qln  *quicListener
	srv  *http.Server
	Port int
}

func NewReverseProxy(
	addr string,
	pemEncodedCert, pemEncodedPrivKey []byte,
	verbose bool,
	errChan chan<- error,
) (*QuicReverseProxy, error) {
	if addr == "" {
		return nil, log.Errorf("Required addr is empty")
	}
	if pemEncodedCert == nil || pemEncodedPrivKey == nil {
		return nil, log.Errorf("Required pemEncodedCert or pemEncodedPrivKey are empty")
	}

	p := goproxy.NewProxyHttpServer()
	p.Verbose = verbose
	p.NonproxyHandler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// TODO <28-02-22, soltzen> This occurs when a non-CONNECT HTTP verb is
		// used (e.g., GET, POST, etc.). Should we blackhole this to obfuscate
		// the free peer?
		w.WriteHeader(http.StatusTeapot)
	})
	p.ConnectDial = func(ctx context.Context, network, addr string) (net.Conn, error) {
		log.Debugf("reverse proxy: Dialing %v", addr)
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}

	if p.Verbose {
		// Print the request we're proxying, just for debugging purposes
		p.OnRequest().HandleConnectFunc(
			func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
				log.Debugf("Reverse proxy: Received CONNECT request for host %s", host)
				return goproxy.OkConnect, host
			})
	}

	// Listen with these certs
	tlsCert, err := tls.X509KeyPair(pemEncodedCert, pemEncodedPrivKey)
	if err != nil {
		return nil, log.Errorf("while X509KeyPair %v", err)
	}
	ln, err := newQuicListener(
		addr, []tls.Certificate{tlsCert},
	)
	if err != nil {
		return nil, log.Errorf(": %v", err)
	}
	srv := &http.Server{Addr: addr, Handler: p}
	go func() {
		err := srv.Serve(ln)
		if err != nil && err != ErrQuicListenerClosed && errChan != nil {
			errChan <- log.Errorf(": %v", err)
		}
	}()
	return &QuicReverseProxy{
		qln:  ln,
		srv:  srv,
		Port: ln.Addr().(*net.UDPAddr).Port,
	}, nil
}

func (p *QuicReverseProxy) Shutdown(ctx context.Context) error {
	close(p.qln.connChan)
	p.qln.Close()
	return p.srv.Shutdown(ctx)
}

type quicListener struct {
	quic.Listener
	connChan chan net.Conn
}

func newQuicListener(
	addr string,
	tlsCerts []tls.Certificate,
) (*quicListener, error) {
	ln, err := quic.ListenAddr(addr, &tls.Config{
		Certificates: tlsCerts,
		NextProtos:   SupportedTLSProtos,
	}, nil)
	if err != nil {
		return nil, log.Errorf(": %v", err)
	}
	ql := &quicListener{
		Listener: ln,
		// Accept up to 20 connections at a time
		connChan: make(chan net.Conn, 20),
	}
	go ql.runAcceptRoutine()
	return ql, nil
}

func (ql *quicListener) runAcceptRoutine() {
	for {
		// Accept a session
		// TODO <21-02-22, soltzen> Figure out a context
		sess, err := ql.Listener.Accept(context.TODO())
		if err != nil {
			if strings.Contains(err.Error(), "server closed") {
				return
			}
			log.Errorf("reverse proxy: Failed to accept session: %v", err)
			continue
		}
		log.Debugf("reverse proxy: Accepted a session. Proceeding to accept streams")
		go func(sess quic.Session) {
			for {
				// TODO <21-02-22, soltzen> Figure out a context
				stream, err := sess.AcceptStream(context.TODO())
				if err != nil {
					// XXX <09-02-22, soltzen> This happens if you shutdown a
					// stream. That's normal:
					// https://github.com/lucas-clemente/quic-go/blob/b935a54c295e2a91e21302c3e48cb46646d635b8/session.go#L1458
					if strings.Contains(err.Error(), "Application error 0x0") {
						return
					}
					log.Errorf(
						"accept stream failed for addr [%v]: %v",
						sess.RemoteAddr(), err)
					sess.CloseWithError(500, "AcceptStream error")
					return
				}
				log.Debugf(
					"reverse proxy: Accepted a stream with ID %v for remote addr %v",
					stream.StreamID(), sess.RemoteAddr())
				ql.connChan <- &QuicConn{Sess: sess, Stream: stream}
			}
		}(sess)
	}
}

// Blocks until there's a new connection
func (ql *quicListener) Accept() (net.Conn, error) {
	c, ok := <-ql.connChan
	if !ok {
		// XXX <09-02-22, soltzen> You'll see this error returned from
		// QuicReverseProxy.srv.Serve()
		return nil, ErrQuicListenerClosed
	}
	return c, nil
}
