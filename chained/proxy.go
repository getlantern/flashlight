package chained

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	pt "git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/obfs4.git/transports/base"
	"git.torproject.org/pluggable-transports/obfs4.git/transports/obfs4"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/buffers"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/keyman"
	"github.com/getlantern/lampshade"
	"github.com/getlantern/mtime"
	"github.com/getlantern/netx"
	"github.com/getlantern/tlsdialer"
)

const (
	trustedSuffix = " (t)"
)

var (
	chainedDialTimeout          = 10 * time.Second
	theForceAddr, theForceToken string
)

// Proxy represents a proxy Lantern client can connect to.
type Proxy interface {
	// Proxy server's protocol (http, https or obfs4)
	Protocol() string
	// Proxy server's network (tcp)
	Network() string
	// Proxy server's address in host:port format
	Addr() string
	// A human friendly string to identify the proxy
	Label() string
	// Is it ok to proxy non-encrypted traffic over it?
	Trusted() bool
	// How can we dial this proxy directly
	DialServer() (net.Conn, error)
	// Adapt HTTP request sent over to the proxy
	AdaptRequest(*http.Request)
}

// CreateProxy creates a Proxy with supplied server info.
func CreateProxy(name string, s *ChainedServerInfo) (Proxy, error) {
	if theForceAddr != "" && theForceToken != "" {
		forceProxy(s)
	}
	if s.Addr == "" {
		return nil, errors.New("Empty addr")
	}
	if s.AuthToken == "" {
		return nil, errors.New("No auth token").With("addr", s.Addr)
	}
	switch s.PluggableTransport {
	case "":
		var base Proxy
		var err error
		if s.Cert == "" {
			log.Errorf("No Cert configured for %s, will dial with plain tcp", s.Addr)
			base = newHTTPProxy(name, s)
		} else {
			log.Tracef("Cert configured for  %s, will dial with tls", s.Addr)
			base, err = newHTTPSProxy(name, s)
		}
		return base, err
	case "obfs4":
		return newOBFS4Wrapper(newHTTPProxy(name, s), s)
	case "lampshade":
		return newLampshadeProxy(name, s)
	default:
		return nil, errors.New("Unknown transport").With("addr", s.Addr).With("plugabble-transport", s.PluggableTransport)
	}
}

// ForceProxy forces everything through the HTTP proxy at forceAddr using
// forceToken.
func ForceProxy(forceAddr string, forceToken string) {
	log.Debugf("Forcing proxying through proxy at %v using token %v", forceAddr, forceToken)
	theForceAddr, theForceToken = forceAddr, forceToken
}

func forceProxy(s *ChainedServerInfo) {
	s.Addr = theForceAddr
	s.AuthToken = theForceToken
	s.Cert = ""
	s.PluggableTransport = ""
}

type httpProxy struct {
	BaseProxy
}

func newHTTPProxy(name string, s *ChainedServerInfo) Proxy {
	return &httpProxy{BaseProxy: BaseProxy{name: name, protocol: "http", network: "tcp", addr: s.Addr, authToken: s.AuthToken, trusted: false}}
}

func (d httpProxy) DialServer() (net.Conn, error) {
	op := ops.Begin("dial_to_chained").ChainedProxy(d.Addr(), d.Protocol(), d.Network())
	defer op.End()
	elapsed := mtime.Stopwatch()
	conn, err := netx.DialTimeout("tcp", d.Addr(), chainedDialTimeout)
	op.DialTime(elapsed, err)
	return conn, op.FailIf(err)
}

type httpsProxy struct {
	BaseProxy
	x509cert     *x509.Certificate
	sessionCache tls.ClientSessionCache
}

func newHTTPSProxy(name string, s *ChainedServerInfo) (Proxy, error) {
	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", s.Addr))
	}
	return &httpsProxy{
		BaseProxy:    BaseProxy{name: name, protocol: "https", network: "tcp", addr: s.Addr, authToken: s.AuthToken, trusted: s.Trusted},
		x509cert:     cert.X509(),
		sessionCache: tls.NewLRUClientSessionCache(1000),
	}, nil
}

func (d httpsProxy) DialServer() (net.Conn, error) {
	op := ops.Begin("dial_to_chained").ChainedProxy(d.Addr(), d.Protocol(), d.Network())
	defer op.End()

	elapsed := mtime.Stopwatch()
	conn, err := tlsdialer.DialTimeout(overheadDialer(false, netx.DialTimeout), chainedDialTimeout,
		"tcp", d.Addr(), false, &tls.Config{
			ClientSessionCache: d.sessionCache,
			InsecureSkipVerify: true,
		})
	op.DialTime(elapsed, err)
	if err != nil {
		return nil, op.FailIf(err)
	}
	if !conn.ConnectionState().PeerCertificates[0].Equal(d.x509cert) {
		if closeErr := conn.Close(); closeErr != nil {
			log.Debugf("Error closing chained server connection: %s", closeErr)
		}
		return nil, op.FailIf(log.Errorf("Server's certificate didn't match expected! Server had\n%v\nbut expected:\n%v",
			conn.ConnectionState().PeerCertificates[0], d.x509cert))
	}
	return overheadWrapper(true)(conn, op.FailIf(err))
}

type obfs4Wrapper struct {
	Proxy
	trusted bool
	cf      base.ClientFactory
	args    interface{}
}

func newOBFS4Wrapper(p Proxy, s *ChainedServerInfo) (Proxy, error) {
	if s.Cert == "" {
		return nil, fmt.Errorf("No Cert configured for obfs4 server, can't connect")
	}

	cf, err := (&obfs4.Transport{}).ClientFactory("")
	if err != nil {
		return nil, log.Errorf("Unable to create obfs4 client factory: %v", err)
	}

	ptArgs := &pt.Args{}
	ptArgs.Add("cert", s.Cert)
	ptArgs.Add("iat-mode", s.PluggableTransportSettings["iat-mode"])

	args, err := cf.ParseArgs(ptArgs)
	if err != nil {
		return nil, log.Errorf("Unable to parse client args: %v", err)
	}
	return obfs4Wrapper{p, s.Trusted, cf, args}, nil
}

func (p obfs4Wrapper) Protocol() string {
	return "obfs4"
}

func (p obfs4Wrapper) Trusted() bool {
	// override the trusted flag of wrapped proxy.
	return p.trusted
}

func (p obfs4Wrapper) Label() string {
	label := p.Proxy.Label()
	if p.trusted && !strings.HasSuffix(label, trustedSuffix) {
		label = label + trustedSuffix
	}
	return label
}

func (p obfs4Wrapper) DialServer() (net.Conn, error) {
	op := ops.Begin("dial_to_chained").ChainedProxy(p.Addr(), p.Protocol(), p.Network())
	defer op.End()
	elapsed := mtime.Stopwatch()
	dialFn := func(network, address string) (net.Conn, error) {
		// We know for sure the network and address are the same as what
		// the inner DailServer uses.
		return p.Proxy.DialServer()
	}
	// The proxy it wrapped already has timeout applied.
	conn, err := p.cf.Dial("tcp", p.Addr(), dialFn, p.args)
	op.DialTime(elapsed, err)
	return overheadWrapper(true)(conn, op.FailIf(err))
}

type lampshadeProxy struct {
	BaseProxy
	dial func() (net.Conn, error)
}

func newLampshadeProxy(name string, s *ChainedServerInfo) (Proxy, error) {
	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", s.Addr))
	}
	cipherCode := lampshade.Cipher(s.ptSettingInt(fmt.Sprintf("cipher_%v", runtime.GOARCH)))
	if cipherCode == 0 {
		if runtime.GOARCH == "amd64" {
			// On 64-bit Intel, default to AES128_GCM which is hardware accelerated
			cipherCode = lampshade.AES128GCM
		} else {
			// default to ChaCha20Poly1305 which is fast even without hardware acceleration
			cipherCode = lampshade.ChaCha20Poly1305
		}
	}
	windowSize := s.ptSettingInt("windowsize")
	maxPadding := s.ptSettingInt("maxpadding")
	maxStreamsPerConn := uint16(s.ptSettingInt("streams"))
	pingInterval, parseErr := time.ParseDuration(s.ptSetting("pinginterval"))
	if parseErr != nil || pingInterval <= 0 {
		log.Debug("Defaulting pinginterval to 15 seconds")
		pingInterval = 15 * time.Second
	}
	dialer := lampshade.NewDialer(windowSize, maxPadding, maxStreamsPerConn, pingInterval, buffers.Pool, cipherCode, cert.X509().PublicKey.(*rsa.PublicKey), func() (net.Conn, error) {
		op := ops.Begin("dial_to_chained").ChainedProxy(s.Addr, "lampshade", "tcp").
			Set("ls_win", windowSize).
			Set("ls_pad", maxPadding).
			Set("ls_streams", int(maxStreamsPerConn)).
			Set("ls_cipher", cipherCode.String())
		defer op.End()

		elapsed := mtime.Stopwatch()
		conn, err := netx.DialTimeout("tcp", s.Addr, chainedDialTimeout)
		op.DialTime(elapsed, err)
		return overheadWrapper(false)(conn, op.FailIf(err))
	})
	dial := func() (net.Conn, error) {
		return overheadWrapper(true)(dialer.Dial())
	}

	proxy := &lampshadeProxy{
		BaseProxy: BaseProxy{name: name, protocol: "lampshade", network: "tcp", addr: s.Addr, authToken: s.AuthToken, trusted: s.Trusted},
		dial:      dial,
	}

	go func() {
		for {
			time.Sleep(pingInterval * 2)
			ttfa := dialer.EMARTT()
			log.Debugf("%v EMA RTT: %v", proxy.Label(), ttfa)
		}
	}()

	return proxy, nil
}

func (d lampshadeProxy) DialServer() (net.Conn, error) {
	return d.dial()
}

type BaseProxy struct {
	name      string
	protocol  string
	network   string
	addr      string
	trusted   bool
	authToken string
}

func (p BaseProxy) Protocol() string {
	return p.protocol
}

func (p BaseProxy) Network() string {
	return p.network
}

func (p BaseProxy) Addr() string {
	return p.addr
}

func (p BaseProxy) Label() string {
	label := fmt.Sprintf("%-38v at %21v", p.name, p.addr)
	if p.trusted {
		label = label + trustedSuffix
	}
	return label
}

func (p BaseProxy) Trusted() bool {
	return p.trusted
}

func (p BaseProxy) DialServer() (net.Conn, error) {
	panic("should implement DialServer")
}

func (p BaseProxy) AdaptRequest(req *http.Request) {
	req.Header.Add("X-Lantern-Auth-Token", p.authToken)
}
