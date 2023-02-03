package quicproxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/elazarl/goproxy"
	"github.com/lucas-clemente/quic-go"
)

// QuicForwardProxy is an HTTP CONNECT proxy that uses QUIC protocol for
// dialing. Use it to communicate with a quicproxy.QuicReverseProxy.
//
// In other words, this proxy converts HTTP traffic (from a TCP connection) to
// QUIC.
//
// The ingress traffic to this proxy is regular HTTP through TCP.
// The egress traffic of this proxy is QUIC.
//
// HTTP3 is not used here.
type QuicForwardProxy struct {
	// Unlike what the type name suggests, this actually implements an
	// *http.Handler, **not** http.Server
	Proxy *goproxy.ProxyHttpServer
	Port  int

	// Always in the form of ip:port
	revProxyAddr string
	srv          *http.Server
	Dialer       *quicDialer
}

func (qfp *QuicForwardProxy) SetReverseProxyUrl(revProxyUrl string) {
	Log.Infof("Setting reverse proxy URL to %s", revProxyUrl)
	// Forward any CONNECT requests to this proxy to the reverse proxy
	// As a reminder, here's how the flow goes:
	//
	//     CensoredPeer fires HTTPS request
	//     -> forward proxy (in CensoredPeer's local machine)
	//       -> request travels through the internet
	//         -> To the reverse proxy on a remote machine (in the FreePeer's machine)
	//           -> Which relays it to the internet
	//
	qfp.Proxy.ConnectDial = qfp.Proxy.NewConnectDialToProxyWithHandler(
		revProxyUrl,
		func(originalConnectReq, newConnectReq *http.Request) {
			// Forward no-metrics headers, if any
			if originalConnectReq.Header.Get(goproxy.GoproxyNoMetricsHeader) != "" {
				newConnectReq.Header.Add(goproxy.GoproxyNoMetricsHeader, "1")
			}
		},
	)
	qfp.revProxyAddr = strings.TrimPrefix(revProxyUrl, "http://")
}

func NewForwardProxy(
	verbose bool,
	insecureSkipVerify bool,
	quicConfig *quic.Config,
) (*QuicForwardProxy, error) {
	// Make a new proxy that uses a QUIC dialer and proxies traffic to a reverseProxyUrl
	qfp := &QuicForwardProxy{}
	p := goproxy.NewProxyHttpServer()
	p.ID = "forwardproxy"
	p.Logger = Log
	p.Verbose = verbose
	// TODO <02-06-2022, soltzen> For whatever reason, when using DialContext
	// instead of this (and changing the Dial in QuicDialer to be DialContext),
	// proxying HTTPS requests fails miserably. You can reproduce this by
	// using DialContext and running the test suite
	qfp.Dialer = NewQuicDialer(
		insecureSkipVerify,
		quicConfig,
		func() string {
			return qfp.revProxyAddr
		})
	p.Tr.Dial = qfp.Dialer.Dial

	// TODO <11-02-22, soltzen> Add cert pinning later
	// p.Tr.DialTLS = dialTlsAndCheckPinnedCert(
	// 	reverseProxyPubKeyHash,
	// 	&tls.Config{InsecureSkipVerify: false},
	// )
	// Log incoming CONNECT requests
	p.OnRequest().HandleConnectFunc(
		func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
			Log.Infof(
				"Forward proxy: Received CONNECT request for host %s",
				host,
			)
			return goproxy.OkConnect, host
		})

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

	qfp.Proxy = p
	return qfp, nil
}

func (p *QuicForwardProxy) Run(port int, errChan chan<- error) error {
	// Forward proxy is always on localhost
	addr := "localhost:" + strconv.Itoa(port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetListen, err)
	}
	srv := &http.Server{Addr: addr, Handler: p.Proxy}
	go func() {
		err := srv.Serve(ln)
		if err != nil &&
			err != http.ErrServerClosed &&
			errChan != nil {
			errChan <- fmt.Errorf("%w: %v", ErrServerServe, err)
		}
	}()
	p.srv = srv
	p.Port = ln.Addr().(*net.TCPAddr).Port
	return nil
}

// func dialTlsAndCheckPinnedCert(
// 	fingerprint []byte,
// 	tlsConfig *tls.Config) func(string, string) (net.Conn, error) {
// 	return func(network, addr string) (net.Conn, error) {
// 		conn, err := tls.Dial(network, addr, tlsConfig)
// 		if err != nil {
// 			return conn, err
// 		}
// 		connstate := conn.ConnectionState()
// 		didValidateCert := false
// 		for _, peercert := range connstate.PeerCertificates {
// 			// Get public key of cert
// 			der, err := x509.MarshalPKIXPublicKey(peercert.PublicKey)
// 			if err != nil {
// 				return nil, err
// 			}
// 			// hash it
// 			hash := sha256.Sum256(der)
// 			if bytes.Compare(hash[:], fingerprint) == 0 {
// 				didValidateCert = true
// 			}
// 		}
// 		if didValidateCert {
// 			return conn, nil
// 		} else {
//			return nil, fmt.Errorf("%w; [fingerprint:%v]; %v", ErrPinnedCertNotFound, fingerprint, err)
// 		}
// 	}
// }

type quicDialer struct {
	// Fetches the address of the reverse proxy we're about to talk to.
	// This is necessary mainly because of the difference between how HTTPS and
	// HTTP proxying works:
	// - HTTPS: When proxying, the client fires a CONNECT request to the reverse
	//   proxy, which is then proxied to the remote server. The remote server
	//   responds with a 200 OK, and the reverse proxy responds with a 200 OK.
	//   The reverse proxy then forwards the request to the remote server.
	// - HTTP: When proxying, the client fires the SAME request (GET/POST/etc)
	//   to the proxy server (no CONNECT is done here). The proxy server then
	//   forwards the request to the remote server directly.
	//
	// Because of this difference, we will have the reverse proxy address (as
	// "addr") in DialContext below **only** in the case of HTTPS proxying, not
	// with HTTP proxying, where we'll just have the address of the remote
	// server.
	//
	// Because of this, it's easier to have an explicit getter for the reverse
	// proxy address. This makes this component more coupled to this project,
	// but that's fine.
	//
	// Ref: https://stackoverflow.com/a/34268925/3870025
	fetchReverseProxyAddr func() string

	// Can be nil. If so, quic-go will use the default config
	quicConfig         *quic.Config
	conn               quic.Connection
	insecureSkipVerify bool
	sync.Mutex
}

func NewQuicDialer(
	insecureSkipVerify bool,
	quicConfig *quic.Config,
	fetchReverseProxyAddr func() string,
) *quicDialer {
	return &quicDialer{
		insecureSkipVerify:    insecureSkipVerify,
		quicConfig:            quicConfig,
		fetchReverseProxyAddr: fetchReverseProxyAddr,
	}
}

func (d *quicDialer) DialContext(
	ctx context.Context,
	network, addr string,
) (net.Conn, error) {
	d.Lock()
	defer d.Unlock()

	// See comment in quicDialer.fetchReverseProxyAddr
	addr = d.fetchReverseProxyAddr()
	if addr == "" {
		return nil, ErrNoReverseProxyAddr
	}

	var err error
	var stream quic.Stream
	conn := d.conn
	if conn != nil {
		Log.Infof("Attempting to reuse old session for addr [%v]", addr)
		stream, err = openQuicStream(ctx, conn)
		if err != nil {
			Log.Infof(
				"Failed to open stream for existing session with err [%v]: Remaking session...",
				err,
			)
			conn, stream, err = makeNewQuicConnAndOpenStream(
				ctx,
				addr,
				d.insecureSkipVerify,
				d.quicConfig,
			)
			if err != nil {
				Log.Infof("Failed to open new stream for addr [%v]", addr)
				return nil, err
			}
		}
	} else {
		conn, stream, err = makeNewQuicConnAndOpenStream(ctx, addr, d.insecureSkipVerify, d.quicConfig)
		if err != nil {
			Log.Infof("Failed to open new stream for addr [%v]", addr)
			return nil, err
		}
	}
	d.conn = conn
	Log.Infof(
		"Established stream [%v] for addr [%v] successfully",
		stream.StreamID(),
		addr,
	)
	return &QuicConn{Sess: d.conn, Stream: stream}, nil
}

func (d *quicDialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.TODO(), network, addr)
}

func openQuicStream(
	ctx context.Context,
	conn quic.Connection,
) (quic.Stream, error) {
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		err = fmt.Errorf("%w: %v", ErrQuicListenerClosed, err)
		conn.CloseWithError(500, err.Error())
		return nil, err
	}
	return stream, nil
}

func makeNewQuicConnAndOpenStream(
	ctx context.Context,
	addr string,
	insecureSkipVerify bool,
	quicConfig *quic.Config,
) (quic.Connection, quic.Stream, error) {
	sess, err := quic.DialAddrContext(ctx, addr, &tls.Config{
		InsecureSkipVerify: insecureSkipVerify,
		NextProtos:         []string{"quic-proxy"},
	}, quicConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrQuicDialAddr, err)
	}
	stream, err := openQuicStream(ctx, sess)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrQuicOpenStream, err)
	}
	return sess, stream, nil
}

func (qfp *QuicForwardProxy) Shutdown(ctx context.Context) error {
	if qfp.Dialer.conn != nil {
		// 0 == No Error
		err := qfp.Dialer.conn.CloseWithError(0, "")
		if err != nil {
			return fmt.Errorf("%w: %v", ErrQuicCloseWithError, err)
		}
	}
	if qfp.srv != nil {
		err := qfp.srv.Shutdown(ctx)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrQuicCloseWithError, err)
		}
	}
	return nil
}
