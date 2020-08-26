package chained

import (
	"context"
	stls "crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"net"
	"strings"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/hellosplitter"
	"github.com/getlantern/netx"
	"github.com/getlantern/tlsmasq"
	"github.com/getlantern/tlsmasq/ptlshs"
)

type tlsMasqImpl struct {
	nopCloser
	addr                    string
	cfg                     tlsmasq.DialerConfig
	tlsClientHelloSplitting bool
}

func newTLSMasqImpl(name, addr string, s *ChainedServerInfo, uc common.UserConfig) (proxyImpl, error) {

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
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, errors.New("failed to load system cert pool: %v", err)
	}
	pool.AddCert(cert)

	pCfg, helloID := tlsConfigForProxy(name, s, uc)
	pCfg.ServerName = sni
	pCfg.InsecureSkipVerify = InsecureSkipVerifyTLSMasqOrigin

	cfg := tlsmasq.DialerConfig{
		ProxiedHandshakeConfig: ptlshs.DialerConfig{
			Handshaker: utlsHandshaker{pCfg, helloID},
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

	return &tlsMasqImpl{addr: addr, cfg: cfg, tlsClientHelloSplitting: s.TLSClientHelloSplitting}, nil
}

func (impl *tlsMasqImpl) dialServer(op *ops.Op, ctx context.Context, dialCore dialCoreFn) (net.Conn, error) {
	tcpConn, err := dialCore(op, ctx)
	if err != nil {
		return nil, err
	}
	if impl.tlsClientHelloSplitting {
		tcpConn = hellosplitter.Wrap(tcpConn, splitClientHello)
	}
	conn := tlsmasq.Client(tcpConn, impl.cfg)

	// We execute the handshake as part of the dial. Otherwise, preconnecting wouldn't do
	// much for us.
	errc := make(chan error)
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

func (impl *tlsMasqImpl) dialCore(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return netx.DialTimeout("tcp", impl.addr, timeoutFor(ctx))
}
