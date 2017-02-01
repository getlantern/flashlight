package chained

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	pt "git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/obfs4.git/transports/base"
	"git.torproject.org/pluggable-transports/obfs4.git/transports/obfs4"
	kcp "github.com/xtaci/kcp-go"

	"github.com/getlantern/cmux"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/keyman"
	"github.com/getlantern/mtime"
	"github.com/getlantern/netx"
	"github.com/getlantern/snappyconn"
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
	// Proxy server's network (tcp or kcp)
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
	case "obfs4-kcp":
		return newOBFS4Wrapper(newKCPProxy(name, s), s)
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
	return wrapOverhead(false, conn), op.FailIf(err)
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
	return wrapOverhead(true, conn), op.FailIf(err)
}

type kcpProxy struct {
	BaseProxy
	dialFN func(network, address string) (net.Conn, error)
}

func newKCPProxy(name string, s *ChainedServerInfo) Proxy {
	return &kcpProxy{
		BaseProxy: BaseProxy{name: name, protocol: "obfs4", network: "kcp", addr: s.Addr, authToken: s.AuthToken, trusted: false},
		// TODO: parameterize inputs to KCP
		dialFN: cmux.Dialer(&cmux.DialerOpts{Dial: dialKCP}),
	}
}

func (p kcpProxy) DialServer() (net.Conn, error) {
	return p.dialFN(p.Network(), p.Addr())
}

func dialKCP(network, address string) (net.Conn, error) {
	block, err := kcp.NewNoneBlockCrypt(nil)
	if err != nil {
		return nil, errors.New("Unable to initialize AES-128 cipher: %v", err)
	}
	// TODO: the below options are hardcoded based on the defaults in kcptun.
	// At some point, it would be nice to make these tunable via the server pt
	// properties, but these defaults work well for now.
	conn, err := kcp.DialWithOptions(address, block, 10, 3)
	if err != nil {
		return nil, err
	}
	conn.SetStreamMode(true)
	conn.SetNoDelay(0, 20, 2, 1)
	conn.SetWindowSize(128, 1024)
	conn.SetMtu(1350)
	conn.SetACKNoDelay(false)
	conn.SetKeepAlive(10)
	conn.SetDSCP(0)
	conn.SetReadBuffer(4194304)
	conn.SetWriteBuffer(4194304)
	return snappyconn.Wrap(conn), nil
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
	return wrapOverhead(true, conn), op.FailIf(err)
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
