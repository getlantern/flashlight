package chained

import (
	"context"
	"crypto/x509"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/hellosplitter"
	"github.com/getlantern/keyman"
	"github.com/getlantern/tlsdialer/v3"
	tls "github.com/refraction-networking/utls"
)

type httpsImpl struct {
	nopCloser
	dialCore                coreDialer
	addr                    string
	certPEM                 string
	x509cert                *x509.Certificate
	tlsConfig               *tls.Config
	roller                  *helloRoller
	tlsClientHelloSplitting bool
	sync.Mutex
}

func newHTTPSImpl(configDir, name, addr string, s *ChainedServerInfo, uc common.UserConfig, dialCore coreDialer) (proxyImpl, error) {
	const timeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", addr))
	}
	tlsConfig, hellos := tlsConfigForProxy(configDir, ctx, name, s, uc)
	if len(hellos) == 0 {
		return nil, log.Error(errors.New("expected at least one hello"))
	}
	return &httpsImpl{
		dialCore:                dialCore,
		addr:                    addr,
		certPEM:                 string(cert.PEMEncoded()),
		x509cert:                cert.X509(),
		tlsConfig:               tlsConfig,
		roller:                  &helloRoller{hellos: hellos},
		tlsClientHelloSplitting: s.TLSClientHelloSplitting,
	}, nil
}

func (impl *httpsImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	r := impl.roller.getCopy()
	defer impl.roller.updateTo(r)

	currentHello := r.current()
	d := tlsdialer.Dialer{
		DoDial: func(network, addr string, timeout time.Duration) (net.Conn, error) {
			tcpConn, err := impl.dialCore(op, ctx, impl.addr)
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
		Config:         impl.tlsConfig.Clone(),
		ClientHelloID:  currentHello.id,
		// TODO: clone currentHello.spec: https://github.com/getlantern/flashlight/issues/1038
		ClientHelloSpec: currentHello.spec,
	}
	result, err := d.DialForTimings("tcp", impl.addr)
	if err != nil {
		if strings.Contains(err.Error(), "tls: ") ||
			strings.Contains(err.Error(), "failed to apply custom client hello spec: ") {
			// A TLS-level error is likely related to a bad hello.
			log.Debugf("got error likely related to bad hello; advancing roller: %v", err)
			r.advance()
		}
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
