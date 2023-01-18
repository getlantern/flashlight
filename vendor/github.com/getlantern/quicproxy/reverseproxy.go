package quicproxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/elazarl/goproxy"
	"github.com/lucas-clemente/quic-go"
)

var (
	SupportedTLSProtos = []string{"quic-proxy"}
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

// Create a new ReverseProxy
//
// bytesReadCallback and bytesWrittenCallback are optional. If they are not nil,
// they will be called with the number of bytes read and written to the
// connection.
//
// a metricsHandler is optional. If metricsHandler is not nil, it will be
// called when the proxy is hit with a an HTTP (not HTTPS) request before we
// proxy it. This is useful for implementing a /metrics endpoint.
//
// An allowedDomainHandler is optional. If allowedDomainHandler is not nil,
// it'll be called with the domain name of the request (e.g.
// "bunnyfoofoo.com"). If it returns false, the request will be
// rejected with a 403 StatusForbidden.
func NewReverseProxy(
	addr string,
	pemEncodedCert, pemEncodedPrivKey []byte,
	verbose bool,
	errChan chan<- error,
	bytesReadCallback, bytesWrittenCallback func(string, int64),
	metricsHandler func(w http.ResponseWriter, req *http.Request) int64,
	allowedDomainHandler func(domain string) bool,
) (*QuicReverseProxy, error) {
	if addr == "" {
		return nil, ErrAddrNotFound
	}
	if pemEncodedCert == nil || pemEncodedPrivKey == nil {
		return nil, ErrCertOrPrivKeyNotFound
	}

	p := goproxy.NewProxyHttpServer()
	p.ID = "reverseproxy"
	p.Logger = Log
	p.Verbose = verbose
	p.BytesReadCallback = bytesReadCallback
	p.BytesWrittenCallback = bytesWrittenCallback

	// This is where HTTP (not HTTPS) traffic is handled.
	// See comment in quicDialer.fetchReverseProxyAddr and the README's
	// "Why is proxying HTTPS ..." for more info
	p.NonproxyHandler = func(w http.ResponseWriter, req *http.Request) int64 {
		// Check if our metricsHandler callback can handle this request. If
		// so, return.
		// Else, continue proxying the request.
		if req.URL.Path == "/metrics" && metricsHandler != nil {
			// No need to return anything since metricsHandler already
			// wrote the response.
			return metricsHandler(w, req)
		}
		if allowedDomainHandler != nil &&
			!allowedDomainHandler(req.Host) {
			w.WriteHeader(http.StatusForbidden)
			return 0
		}

		// Clone the request.
		req, err := http.NewRequestWithContext(
			req.Context(),
			req.Method,
			// Reassemble the HTTP request from the Host header
			"http://"+req.Host+req.URL.String(),
			req.Body,
		)
		if err != nil {
			Log.Errorf(
				"reverse proxy: Failed to create new request: %v",
				err,
			)
			return 0
		}

		// Launch the request to the remote server, copy the headers and the
		// body and return the status code
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			Log.Errorf("reverse proxy: Failed to make request: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return 0
		}
		defer resp.Body.Close()
		for k, v := range resp.Header {
			w.Header().Add(k, v[0])
		}
		n, _ := io.Copy(w, resp.Body)
		w.WriteHeader(resp.StatusCode)
		return n
	}

	p.ConnectDial = func(originalConnectReq *http.Request, network, addr string) (net.Conn, error) {
		// addr here usually looks like "www.bunnyfoofoo.com:443"
		host := strings.Split(addr, ":")[0]
		if allowedDomainHandler != nil &&
			!allowedDomainHandler(host) {
			return nil, fmt.Errorf("domain %s not allowed", host)
		}

		Log.Infof("reverse proxy: Dialing %v", addr)
		var d net.Dialer
		// TODO <05-08-2022, soltzen> Should we do a new context here maybe?
		return d.DialContext(originalConnectReq.Context(), network, addr)
	}

	if p.Verbose {
		p.OnRequest().Do(
			goproxy.FuncReqHandler(func(req *http.Request,
				ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
				Log.Infof(
					"reverse proxy: Proxying request: %s",
					req.URL.String(),
				)
				return req, nil
			}))
		// Print the request we're proxying, just for debugging purposes
		p.OnRequest().HandleConnectFunc(
			func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
				Log.Infof(
					"Reverse proxy: Received CONNECT request for host %s",
					host,
				)
				return goproxy.OkConnect, host
			})
	}

	// Listen to read-write errors
	p.ReadWriteErrChan = make(chan error)
	go func() {
		for {
			err := <-p.ReadWriteErrChan
			if err == nil {
				continue
			}
			if strings.Contains(err.Error(), "Application error 0x0") {
				// XXX <02-06-2022, soltzen> This is a typical quic-go error meaning "NoError":
				// - https://github.com/lucas-clemente/quic-go/blob/b935a54c295e2a91e21302c3e48cb46646d635b8/session.go#L1458
				// - https://github.com/lucas-clemente/quic-go/blob/b935a54c295e2a91e21302c3e48cb46646d635b8/internal/qerr/error_codes.go#L14
				continue
			}
			Log.Errorf("%v", err)
		}
	}()

	// Listen with these certs
	tlsCert, err := tls.X509KeyPair(pemEncodedCert, pemEncodedPrivKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrX509KeyPair, err)
	}
	ln, err := newQuicListener(
		addr, []tls.Certificate{tlsCert},
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNewQuicListener, err)
	}
	srv := &http.Server{Addr: addr, Handler: p}
	go func() {
		err := srv.Serve(ln)
		if err != nil && err != ErrQuicListenerClosed && errChan != nil {
			errChan <- fmt.Errorf("%w: %v", ErrServerServe, err)
		}
	}()
	return &QuicReverseProxy{
		qln:  ln,
		srv:  srv,
		Port: ln.Addr().(*net.UDPAddr).Port,
	}, nil
}

func (p *QuicReverseProxy) Shutdown(ctx context.Context) error {
	if p.qln != nil {
		close(p.qln.connChan)
		p.qln.Close()
	}
	if p.srv != nil {
		return p.srv.Shutdown(ctx)
	}
	return nil
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
		return nil, fmt.Errorf("%w: %v", ErrQuicListenAddr, err)
	}
	ql := &quicListener{
		Listener: ln,
		// Accept up to 20 connections at a time
		connChan: make(chan net.Conn, 20),
	}
	go ql.runAcceptRoutine()
	return ql, nil
}

func acceptIncomingSession(
	sess quic.Connection,
	connChan chan net.Conn,
) {
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
			Log.Errorf(
				"accept stream failed for addr [%v]: %v",
				sess.RemoteAddr(), err)
			err = fmt.Errorf("%w: %v", ErrQuicAcceptStreamFailed, err)
			sess.CloseWithError(500, err.Error())
			return
		}
		Log.Infof(
			"reverse proxy: Accepted a stream with ID %v for remote addr %v",
			stream.StreamID(),
			sess.RemoteAddr(),
		)
		connChan <- &QuicConn{Sess: sess, Stream: stream}
	}
}

func (ql *quicListener) runAcceptRoutine() {
	for {
		// Accept a session
		// TODO <21-02-22, soltzen> Figure out a context
		sess, err := ql.Listener.Accept(context.TODO())
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "server closed") {
				return
			}
			Log.Errorf("reverse proxy: Failed to accept session: %v", err)
			continue
		}
		Log.Infof(
			"reverse proxy: Accepted a session. Proceeding to accept streams",
		)
		go acceptIncomingSession(
			sess,
			ql.connChan,
		)
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
