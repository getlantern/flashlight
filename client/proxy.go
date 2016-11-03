package client

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"time"

	pt "git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/obfs4.git/transports/base"
	"git.torproject.org/pluggable-transports/obfs4.git/transports/obfs4"
	kcp "github.com/xtaci/kcp-go"

	"github.com/getlantern/cmux"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/keyman"
	"github.com/getlantern/netx"
	"github.com/getlantern/snappyconn"
	"github.com/getlantern/tlsdialer"
	"github.com/getlantern/withtimeout"
)

var (
	chainedDialTimeout          = 10 * time.Second
	theForceAddr, theForceToken string
)

// Proxy represents a proxy Lantern client can connect to.
type Proxy interface {
	// Proxy server's network
	Network() string
	// Proxy server's address in host:port format
	Addr() string
	// A human friendly string to identify the proxy
	Label() string
	// How can we dial this proxy directly
	DialServer() (net.Conn, error)
	// Check the reachibility of the proxy
	Check() bool
	// Set extra headers sent along with each proxy request.
	SetExtraHeaders(headers http.Header)
	// Clean up resources, if any
	Close()
}

// CreateProxy creates a Proxy with supplied server info.
func CreateProxy(s *ChainedServerInfo) (Proxy, error) {
	if theForceAddr != "" && theForceToken != "" {
		forceProxy(s)
	}
	// TODO: respect s.AuthToken and s.Trusted when creating proxy instance
	var base Proxy
	var err error
	if s.Cert == "" {
		log.Error("No Cert configured for chained server, will dial with plain tcp")
		base = newHTTPProxy(s)
	} else {
		log.Trace("Cert configured for chained server, will dial with tls")
		base, err = newHTTPSProxy(s)
	}
	switch s.PluggableTransport {
	case "":
		return base, err
	case "obfs4-tcp":
		return newOBFS4Wrapper(newHTTPProxy(s), s)
	case "obfs4-kcp":
		return newOBFS4Wrapper(newKCPProxy(s), s)
	default:
		return nil, errors.New("Unknown transport").With("plugabble-transport", s.PluggableTransport)
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
	baseProxy
}

func newHTTPProxy(s *ChainedServerInfo) Proxy {
	return &httpProxy{baseProxy: baseProxy{network: "tcp", addr: s.Addr}}
}

func (d httpProxy) DialServer() (net.Conn, error) {
	op := ops.Begin("dial_to_chained").ChainedProxy(d.Addr(), "http")
	defer op.End()
	start := time.Now()
	conn, err := netx.DialTimeout("tcp", d.Addr(), chainedDialTimeout)
	op.DialTime(start, err)
	return conn, op.FailIf(err)
}

type httpsProxy struct {
	baseProxy
	x509cert     *x509.Certificate
	sessionCache tls.ClientSessionCache
}

func newHTTPSProxy(s *ChainedServerInfo) (Proxy, error) {
	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Errorf("Unable to parse certificate: %s", err)
	}
	return &httpsProxy{
		baseProxy:    baseProxy{network: "tcp", addr: s.Addr},
		x509cert:     cert.X509(),
		sessionCache: tls.NewLRUClientSessionCache(1000),
	}, nil
}

func (d httpsProxy) DialServer() (net.Conn, error) {
	op := ops.Begin("dial_to_chained").ChainedProxy(d.Addr(), "https")
	defer op.End()

	start := time.Now()
	conn, err := tlsdialer.DialTimeout(netx.DialTimeout, chainedDialTimeout,
		"tcp", d.Addr(), false, &tls.Config{
			ClientSessionCache: d.sessionCache,
			InsecureSkipVerify: true,
		})
	op.DialTime(start, err)
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
	return conn, op.FailIf(err)
}

type kcpProxy struct {
	baseProxy
	dialFN func(network, address string) (net.Conn, error)
}

func newKCPProxy(s *ChainedServerInfo) Proxy {
	return &kcpProxy{
		baseProxy: baseProxy{network: "kcp", addr: s.Addr},
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
	cf   base.ClientFactory
	args interface{}
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
	return obfs4Wrapper{p, cf, args}, nil
}

func (p obfs4Wrapper) Label() string {
	return "obfs4-" + p.Proxy.Label()
}

func (p obfs4Wrapper) DialServer() (net.Conn, error) {
	op := ops.Begin("dial_to_chained").ChainedProxy(p.Addr(), "obfs4")
	defer op.End()
	start := time.Now()
	dialFn := func(network, address string) (net.Conn, error) {
		// We know for sure the network and address are the same as what
		// the inner DailServer uses.
		return p.Proxy.DialServer()
	}
	_conn, _, err := withtimeout.Do(chainedDialTimeout, func() (interface{}, error) {
		return p.cf.Dial("tcp", p.Addr(), dialFn, p.args)
	})
	op.DialTime(start, err)
	var conn net.Conn
	if err == nil {
		conn = _conn.(net.Conn)
	}
	return conn, op.FailIf(err)
}

type obfs4OverTCP struct {
	baseProxy
}

type obfs4OverKCP struct {
	Proxy
}

type nullDialer struct{}

func (d nullDialer) DialServer() (net.Conn, error) {
	panic("should implement DialServer")
	return nil, nil
}

type baseProxy struct {
	extraHeaders http.Header
	network      string
	addr         string
}

func (p baseProxy) DialServer() (net.Conn, error) {
	panic("should implement DialServer")
	return nil, nil
}

func (p baseProxy) Network() string {
	return p.network
}

func (p baseProxy) Addr() string {
	return p.addr
}

func (p baseProxy) Label() string {
	return p.Network() + " " + p.Addr()
}

func (p baseProxy) Check() bool {
	return false
}

func (p baseProxy) SetExtraHeaders(headers http.Header) {
	p.extraHeaders = headers
}

func (p baseProxy) Close() {
}
