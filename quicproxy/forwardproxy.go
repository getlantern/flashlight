package quicproxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/url"
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
	proxy *goproxy.ProxyHttpServer
	srv   *http.Server
	Port  int
}

func (qfp *QuicForwardProxy) SetReverseProxyUrl(revProxyUrl string) {
	log.Debugf("Setting reverse proxy URL to %s", revProxyUrl)
	qfp.proxy.ConnectDial = qfp.proxy.NewConnectDialToProxy(revProxyUrl)
	qfp.proxy.Tr.Proxy = func(req *http.Request) (*url.URL, error) {
		return url.Parse(revProxyUrl)
	}
}

func NewForwardProxy(
	port string,
	// reverseProxyPubKeyHash []byte,
	verbose bool,
	insecureSkipVerify bool,
	errChan chan<- error) (*QuicForwardProxy, error) {
	// Make a new proxy that uses a QUIC dialer and proxies traffic to a reverseProxyUrl
	p := goproxy.NewProxyHttpServer()
	p.Verbose = verbose
	p.Tr.Dial = NewQuicDialer(insecureSkipVerify).Dial
	// TODO <11-02-22, soltzen> Add cert pinning later
	// p.Tr.DialTLS = dialTlsAndCheckPinnedCert(
	// 	reverseProxyPubKeyHash,
	// 	&tls.Config{InsecureSkipVerify: false},
	// )
	// Log incoming requests
	p.OnRequest().Do(
		goproxy.FuncReqHandler(func(req *http.Request,
			ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			log.Debugf("Proxying request: %s", req.URL.String())
			return req, nil
		}))

	// Forward proxy is always on localhost
	addr := "localhost:" + port
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, log.Errorf(" %v", err)
	}
	srv := &http.Server{Addr: addr, Handler: p}
	go func() {
		err := srv.Serve(ln)
		if err != nil &&
			err != http.ErrServerClosed &&
			errChan != nil {
			errChan <- log.Errorf(" %v", err)
		}
	}()
	return &QuicForwardProxy{
		proxy: p,
		srv:   srv,
		Port:  ln.Addr().(*net.TCPAddr).Port,
	}, nil
}

func dialTlsAndCheckPinnedCert(
	fingerprint []byte,
	tlsConfig *tls.Config) func(string, string) (net.Conn, error) {
	return func(network, addr string) (net.Conn, error) {
		conn, err := tls.Dial(network, addr, tlsConfig)
		if err != nil {
			return conn, err
		}
		connstate := conn.ConnectionState()
		didValidateCert := false
		for _, peercert := range connstate.PeerCertificates {
			// Get public key of cert
			der, err := x509.MarshalPKIXPublicKey(peercert.PublicKey)
			if err != nil {
				return nil, err
			}
			// hash it
			hash := sha256.Sum256(der)
			if bytes.Compare(hash[:], fingerprint) == 0 {
				didValidateCert = true
			}
		}
		if didValidateCert {
			return conn, nil
		} else {
			return nil, log.Errorf("Pinned cert with fingerprint %#v was not found in certificate chain", fingerprint)
		}
	}
}

type quicDialer struct {
	insecureSkipVerify bool
	sess               quic.Session
	sync.Mutex
}

func NewQuicDialer(insecureSkipVerify bool) *quicDialer {
	return &quicDialer{
		insecureSkipVerify: insecureSkipVerify,
	}
}

func (d *quicDialer) Dial(network, addr string) (net.Conn, error) {
	openStream := func(sess quic.Session) (quic.Stream, error) {
		stream, err := sess.OpenStreamSync(context.TODO())
		if err != nil {
			s := fmt.Sprintf("while opening stream for %s: %v", addr, err)
			d.sess.CloseWithError(500, s)
			return nil, log.Errorf(s)
		}
		return stream, nil
	}
	makeNewSessionAndOpenStream := func(addr string,
		insecureSkipVerify bool) (quic.Session, quic.Stream, error) {
		sess, err := quic.DialAddr(addr, &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
			NextProtos:         []string{"quic-proxy"},
		}, nil)
		if err != nil {
			return nil, nil, log.Errorf(": %v", err)
		}
		stream, err := openStream(sess)
		if err != nil {
			return nil, nil, log.Errorf(": %v", err)
		}
		d.sess = sess
		return sess, stream, nil
	}

	d.Lock()
	defer d.Unlock()
	var err error
	var stream quic.Stream
	sess := d.sess
	if sess != nil {
		log.Debugf("Attempting to reuse old session for addr [%v]", addr)
		stream, err = openStream(sess)
		if err != nil {
			log.Debugf("Failed to open stream for existing session with err [%v]: Remaking session...", err)
			sess, stream, err = makeNewSessionAndOpenStream(addr, d.insecureSkipVerify)
			if err != nil {
				log.Debugf("Failed to open new stream for addr [%v]", addr)
				return nil, err
			}
		}
	} else {
		sess, stream, err = makeNewSessionAndOpenStream(addr, d.insecureSkipVerify)
		if err != nil {
			log.Debugf("Failed to open new stream for addr [%v]", addr)
			return nil, err
		}
	}
	d.sess = sess
	log.Debugf("Established stream [%v] for addr [%v] successfully", stream.StreamID(), addr)
	return &QuicConn{Sess: d.sess, Stream: stream}, nil
}

func (p *QuicForwardProxy) Shutdown(ctx context.Context) error {
	return p.srv.Shutdown(ctx)
}
