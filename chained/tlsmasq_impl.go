package chained

import (
	"context"
	stls "crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	stderrors "errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/hellosplitter"
	"github.com/getlantern/netx"
	"github.com/getlantern/tlsmasq"
	"github.com/getlantern/tlsmasq/ptlshs"
	"github.com/getlantern/tlsutil"
	tls "github.com/refraction-networking/utls"
)

type tlsMasqImpl struct {
	nopCloser
	reportDialCore          reportDialCoreFn
	addr                    string
	cfg                     tlsmasq.DialerConfig
	tlsClientHelloSplitting bool
}

func newTLSMasqImpl(configDir, name, addr string, pc *config.ProxyConfig, uc common.UserConfig, reportDialCore reportDialCoreFn) (ProxyImpl, error) {
	const timeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	decodeUint16 := func(s string) (uint16, error) {
		b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
		if err != nil {
			return 0, err
		}
		return binary.BigEndian.Uint16(b), nil
	}

	suites := []uint16{}
	suiteStrings := strings.Split(ptSetting(pc, "tlsmasq_suites"), ",")
	if len(suiteStrings) == 1 && suiteStrings[0] == "" {
		return nil, errors.New("no cipher suites specified")
	}
	for _, s := range suiteStrings {
		suite, err := decodeUint16(s)
		if err != nil {
			return nil, errors.New("bad cipher string '%s': %v", s, err)
		}
		suites = append(suites, suite)
	}
	versStr := ptSetting(pc, "tlsmasq_tlsminversion")
	minVersion, err := decodeUint16(versStr)
	if err != nil {
		return nil, errors.New("bad TLS version string '%s': %v", versStr, err)
	}
	secretString := ptSetting(pc, "tlsmasq_secret")
	secretBytes, err := hex.DecodeString(strings.TrimPrefix(secretString, "0x"))
	if err != nil {
		return nil, errors.New("bad server-secret string '%s': %v", secretString, err)
	}
	secret := ptlshs.Secret{}
	if len(secretBytes) != len(secret) {
		return nil, errors.New("expected %d-byte secret string, got %d bytes", len(secret), len(secretBytes))
	}
	copy(secret[:], secretBytes)
	sni := ptSetting(pc, "tlsmasq_sni")
	if sni == "" {
		return nil, errors.New("server name indicator must be configured")
	}
	// It's okay if this is unset - it'll just result in us using the default.
	nonceTTL := time.Duration(ptSettingInt(pc, "tlsmasq_noncettl"))

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, errors.New("malformed server address: %v", err)
	}

	// Add the proxy cert to the root CAs as proxy certs are self-signed.
	if pc.Cert == "" {
		return nil, errors.New("no proxy certificate configured")
	}
	block, rest := pem.Decode([]byte(pc.Cert))
	if block == nil {
		return nil, errors.New("failed to decode proxy certificate as PEM block")
	}
	if len(rest) > 0 {
		return nil, errors.New("unexpected extra data in proxy certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, errors.New("failed to parse proxy certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)

	pCfg, hellos := tlsConfigForProxy(ctx, configDir, name, pc, uc)
	pCfg.ServerName = sni
	pCfg.InsecureSkipVerify = InsecureSkipVerifyTLSMasqOrigin

	cfg := tlsmasq.DialerConfig{
		ProxiedHandshakeConfig: ptlshs.DialerConfig{
			Handshaker: &utlsHandshaker{pCfg, &helloRoller{hellos: hellos}, sync.Mutex{}},
			Secret:     secret,
			NonceTTL:   nonceTTL,
		},
		TLSConfig: &stls.Config{
			MinVersion:   minVersion,
			CipherSuites: suites,
			// Proxy certificates are valid for the host (usually their IP address).
			ServerName: host,
			RootCAs:    pool,
		},
	}

	return &tlsMasqImpl{reportDialCore: reportDialCore, addr: addr, cfg: cfg, tlsClientHelloSplitting: pc.TLSClientHelloSplitting}, nil
}

func (impl *tlsMasqImpl) DialServer(op *ops.Op, ctx context.Context, prefix []byte) (net.Conn, error) {
	tcpConn, err := impl.reportDialCore(op, func() (net.Conn, error) {
		return netx.DialContext(ctx, "tcp", impl.addr)
	})
	if err != nil {
		return nil, err
	}
	if impl.tlsClientHelloSplitting {
		tcpConn = hellosplitter.Wrap(tcpConn, splitClientHello)
	}
	conn := tlsmasq.Client(tcpConn, impl.cfg)

	// We execute the handshake as part of the dial. Otherwise, preconnecting wouldn't do
	// much for us.
	errc := make(chan error, 1)
	go func() { errc <- conn.Handshake() }()
	select {
	case err := <-errc:
		if err != nil {
			conn.Close()
			var alertErr tlsutil.UnexpectedAlertError
			if stderrors.As(err, &alertErr) {
				log.Debugf("received alert from origin in tlsmasq handshake: %v", alertErr.Alert)
			}
			return nil, errors.New("handshake failed: %v", err)
		}
		return conn, nil
	case <-ctx.Done():
		conn.Close()
		return nil, ctx.Err()
	}
}

// utlsHandshaker implements tlsmasq/ptlshs.Handshaker. This allows us to parrot browsers like
// Chrome in our handshakes with tlsmasq origins.
type utlsHandshaker struct {
	cfg    *tls.Config
	roller *helloRoller
	sync.Mutex
}

func (h *utlsHandshaker) Handshake(conn net.Conn) (*ptlshs.HandshakeResult, error) {
	r := h.roller.getCopy()
	defer h.roller.updateTo(r)

	isHelloErr := func(err error) bool {
		// We assume that everything other than timeouts or transient network errors might be
		// related to the hello. This may be a little aggressive, but it's better that the client is
		// willing to try other hellos, rather than get stuck in a loop on a bad one.
		var netErr net.Error
		if !stderrors.As(err, &netErr) {
			return true
		}
		return !netErr.Temporary() && !netErr.Timeout()
	}

	currentHello := r.current()
	uconn, err := currentHello.uconn(conn, h.cfg.Clone())
	if err != nil {
		// An error from helloSpec.uconn implies an invalid hello.
		log.Debugf("invalid custom hello; advancing roller: %v", err)
		r.advance()
		return nil, err
	}
	if err = uconn.Handshake(); err != nil {
		if isHelloErr(err) {
			log.Debugf("got error likely related to bad hello; advancing roller: %v", err)
			r.advance()
		}
		return nil, err
	}
	return &ptlshs.HandshakeResult{
		Version:     uconn.ConnectionState().Version,
		CipherSuite: uconn.ConnectionState().CipherSuite,
	}, nil
}
