package chained

import (
	"context"
	stls "crypto/tls"
	"encoding/binary"
	"encoding/hex"
	stderrors "errors"
	"net"
	"strings"
	"sync"
	"time"

	tls "github.com/refraction-networking/utls"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/ops"
	"github.com/getlantern/hellosplitter"
	"github.com/getlantern/netx"
	"github.com/getlantern/tlsmasq"
	"github.com/getlantern/tlsmasq/ptlshs"
	"github.com/getlantern/tlsutil"
)

type tlsMasqImpl struct {
	nopCloser
	reportDialCore          reportDialCoreFn
	addr                    string
	cfg                     tlsmasq.DialerConfig
	tlsClientHelloSplitting bool
	proxyConfig             *config.ProxyConfig
	connWrappers            []connWrapper
}

func newTLSMasqImpl(configDir, name, addr string, pc *config.ProxyConfig, uc common.UserConfig,
	reportDialCore reportDialCoreFn, connWrappers ...connWrapper) (proxyImpl, error) {

	suites, err := cipherSuites(ptSetting(pc, "tlsmasq_suites"), name)
	if err != nil {
		return nil, errors.New("could not parse suites for: %v", name)
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
	masqSNI := ptSetting(pc, "tlsmasq_sni")
	if masqSNI == "" {
		return nil, errors.New("server name indicator must be configured")
	}
	// It's okay if this is unset - it'll just result in us using the default.
	nonceTTL := time.Duration(ptSettingInt(pc, "tlsmasq_noncettl"))

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, errors.New("malformed server address: %v", err)
	}

	proxyTLS, hellos, err := tlsConfigForProxy(context.Background(), configDir, name, pc, uc)
	if err != nil {
		return nil, errors.New("error generating TLS config: %v", err)
	}

	// For tlsmasq proxies, the TLS config we generated in the previous step is the config used in
	// the initial handshake with the masquerade origin. Thus we need to make some modifications.
	outerTLS := tls.Config{
		CipherSuites:       proxyTLS.CipherSuites,
		KeyLogWriter:       proxyTLS.KeyLogWriter,
		ServerName:         masqSNI,
		InsecureSkipVerify: InsecureSkipVerifyTLSMasqOrigin,
	}

	cfg := tlsmasq.DialerConfig{
		ProxiedHandshakeConfig: ptlshs.DialerConfig{
			Handshaker: &utlsHandshaker{&outerTLS, &helloRoller{hellos: hellos}, sync.Mutex{}},
			Secret:     secret,
			NonceTTL:   nonceTTL,
		},
		// This is the config used for the second, wrapped handshake with the proxy itself.
		TLSConfig: &stls.Config{
			MinVersion:   minVersion,
			CipherSuites: suites,
			// Proxy certificates are valid for the host (usually their IP address).
			ServerName:            host,
			InsecureSkipVerify:    proxyTLS.InsecureSkipVerify,
			VerifyPeerCertificate: proxyTLS.VerifyPeerCertificate,
			RootCAs:               proxyTLS.RootCAs,
			KeyLogWriter:          proxyTLS.KeyLogWriter,
		},
	}

	return &tlsMasqImpl{reportDialCore: reportDialCore,
		addr:                    addr,
		cfg:                     cfg,
		tlsClientHelloSplitting: pc.TLSClientHelloSplitting,
		proxyConfig:             pc,
		connWrappers:            connWrappers,
	}, nil
}

func decodeUint16(s string) (uint16, error) {
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func cipherSuites(cipherSuites, name string) ([]uint16, error) {
	suiteStrings := strings.Split(cipherSuites, ",")
	if len(suiteStrings) == 1 && suiteStrings[0] == "" {
		// No suites specified. Setting them to nil will cause the default suites to be used.
		log.Debugf("No suites specified, using default suites for %s", name)
		return nil, nil
	}
	suites := []uint16{}
	for _, s := range suiteStrings {
		suite, err := decodeUint16(s)
		if err != nil {
			return nil, errors.New("bad cipher string '%s': %v", s, err)
		}
		suites = append(suites, suite)
	}
	return suites, nil
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
	for _, wrapper := range impl.connWrappers {
		tcpConn = wrapper(tcpConn, impl.proxyConfig)
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
			log.Debugf("tlsmasq got error likely related to bad hello; advancing roller: %v", err)
			r.advance()
		}
		return nil, err
	}
	return &ptlshs.HandshakeResult{
		Version:     uconn.ConnectionState().Version,
		CipherSuite: uconn.ConnectionState().CipherSuite,
	}, nil
}
