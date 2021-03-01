package chained

import (
	"context"
	stls "crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/hellosplitter"
	"github.com/getlantern/netx"
	"github.com/getlantern/tlsmasq"
	"github.com/getlantern/tlsmasq/ptlshs"
	tls "github.com/refraction-networking/utls"
)

type tlsMasqImpl struct {
	nopCloser
	reportDialCore          reportDialCoreFn
	addr                    string
	cfg                     tlsmasq.DialerConfig
	tlsClientHelloSplitting bool
}

func newTLSMasqImpl(configDir, name, addr string, s *ChainedServerInfo, uc common.UserConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
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
	suiteStrings := strings.Split(s.ptSetting("tlsmasq_suites"), ",")
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
	versStr := s.ptSetting("tlsmasq_tlsminversion")
	minVersion, err := decodeUint16(versStr)
	if err != nil {
		return nil, errors.New("bad TLS version string '%s': %v", versStr, err)
	}
	secretString := s.ptSetting("tlsmasq_secret")
	secretBytes, err := hex.DecodeString(strings.TrimPrefix(secretString, "0x"))
	if err != nil {
		return nil, errors.New("bad server-secret string '%s': %v", secretString, err)
	}
	secret := ptlshs.Secret{}
	if len(secretBytes) != len(secret) {
		return nil, errors.New("expected %d-byte secret string, got %d bytes", len(secret), len(secretBytes))
	}
	copy(secret[:], secretBytes)
	sni := s.ptSetting("tlsmasq_sni")
	if sni == "" {
		return nil, errors.New("server name indicator must be configured")
	}
	// It's okay if this is unset - it'll just result in us using the default.
	nonceTTL := time.Duration(s.ptSettingInt("tlsmasq_noncettl"))

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, errors.New("malformed server address: %v", err)
	}

	// Add the proxy cert to the root CAs as proxy certs are self-signed.
	if s.Cert == "" {
		return nil, errors.New("no proxy certificate configured")
	}
	block, rest := pem.Decode([]byte(s.Cert))
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

	pCfg, hellos := tlsConfigForProxy(configDir, ctx, name, s, uc)
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

	return &tlsMasqImpl{reportDialCore: reportDialCore, addr: addr, cfg: cfg, tlsClientHelloSplitting: s.TLSClientHelloSplitting}, nil
}

func (impl *tlsMasqImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
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
		if strings.Contains(err.Error(), "hello spec") {
			// These errors are created below.
			return true
		}
		if strings.Contains(err.Error(), "tls: ") {
			// A TLS-level error is likely related to a bad hello.
			return true
		}
		return false
	}

	currentHello := r.current()
	uconn := tls.UClient(conn, h.cfg.Clone(), currentHello.id)
	res, err := func() (*ptlshs.HandshakeResult, error) {
		if currentHello.id == tls.HelloCustom {
			if currentHello.spec == nil {
				return nil, errors.New("hello spec must be provided if HelloCustom is used")
			}
			// TODO: clone currentHello.spec: https://github.com/getlantern/flashlight/issues/1038
			if err := uconn.ApplyPreset(currentHello.spec); err != nil {
				return nil, fmt.Errorf("failed to set custom hello spec: %w", err)
			}
		}
		if err := uconn.Handshake(); err != nil {
			return nil, err
		}
		return &ptlshs.HandshakeResult{
			Version:     uconn.ConnectionState().Version,
			CipherSuite: uconn.ConnectionState().CipherSuite,
		}, nil
	}()
	if err != nil && isHelloErr(err) {
		log.Debugf("got error likely related to bad hello; advancing roller: %v", err)
		r.advance()
	}
	return res, err
}
