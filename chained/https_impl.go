package chained

import (
	"context"
	"crypto/x509"
	"net"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/hellosplitter"
	"github.com/getlantern/keyman"
	"github.com/getlantern/netx"
	"github.com/getlantern/tlsdialer"
	tls "github.com/refraction-networking/utls"
)

type httpsImpl struct {
	nopCloser
	reportDialCore          reportDialCoreFn
	addr                    string
	certPEM                 string
	x509cert                *x509.Certificate
	tlsConfig               *tls.Config
	clientHelloID           tls.ClientHelloID
	tlsClientHelloSplitting bool
}

func newHTTPSImpl(name, addr string, s *ChainedServerInfo, uc common.UserConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", addr))
	}
	tlsConfig, clientHelloID := tlsConfigForProxy(name, s, uc)
	return &httpsImpl{
		reportDialCore:          reportDialCore,
		addr:                    addr,
		certPEM:                 string(cert.PEMEncoded()),
		x509cert:                cert.X509(),
		tlsConfig:               tlsConfig,
		clientHelloID:           clientHelloID,
		tlsClientHelloSplitting: s.TLSClientHelloSplitting,
	}, nil
}

func (impl *httpsImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	td := &tlsdialer.Dialer{
		DoDial: func(network, addr string, timeout time.Duration) (net.Conn, error) {
			tcpConn, err := impl.reportDialCore(op, func() (net.Conn, error) {
				return netx.DialContext(ctx, "tcp", impl.addr)
			})
			if err != nil {
				return nil, err
			}
			if impl.tlsClientHelloSplitting {
				tcpConn = hellosplitter.Wrap(tcpConn, splitClientHello)
			}
			return tcpConn, err
		},
		Timeout:        timeoutFor(ctx),
		SendServerName: impl.tlsConfig.ServerName != "",
		Config:         impl.tlsConfig,
		ClientHelloID:  impl.clientHelloID,
	}
	result, err := td.DialForTimings("tcp", impl.addr)
	if err != nil {
		return nil, err
	}
	conn := result.Conn
	peerCertificates := conn.ConnectionState().PeerCertificates
	// when using tls session resumption from a stored session state, there will be no peer certificates.
	// this is okay.
	resumedSession := len(peerCertificates) == 0
	if !resumedSession && !conn.ConnectionState().PeerCertificates[0].Equal(impl.x509cert) {
		if closeErr := conn.Close(); closeErr != nil {
			log.Debugf("Error closing chained server connection: %s", closeErr)
		}
		var received interface{}
		var expected interface{}
		_received, certErr := keyman.LoadCertificateFromX509(conn.ConnectionState().PeerCertificates[0])
		if certErr != nil {
			log.Errorf("Unable to parse received certificate: %v", certErr)
			received = conn.ConnectionState().PeerCertificates[0]
			expected = impl.x509cert
		} else {
			received = string(_received.PEMEncoded())
			expected = string(impl.certPEM)
		}
		return nil, op.FailIf(log.Errorf("Server's certificate didn't match expected! Server had\n%v\nbut expected:\n%v",
			received, expected))
	}
	return conn, nil
}

func timeoutFor(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if ok {
		return deadline.Sub(time.Now())
	}
	return chainedDialTimeout
}
