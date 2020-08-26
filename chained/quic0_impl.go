package chained

import (
	"context"
	gtls "crypto/tls"
	"net"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/keyman"
	"github.com/getlantern/quic0"
)

type quic0Impl struct {
	addr   string
	dialer *quic0.Client
}

func newQUIC0Impl(name, addr string, s *ChainedServerInfo) (proxyImpl, error) {
	tlsConf := &gtls.Config{
		ServerName:         s.TLSServerNameIndicator,
		InsecureSkipVerify: true,
		KeyLogWriter:       getTLSKeyLogWriter(),
	}

	quicConf := &quic0.Config{
		MaxIncomingStreams: -1,
		KeepAlive:          true,
	}

	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", addr))
	}
	pinnedCert := cert.X509()

	dialFn := quic0.DialWithNetx
	dialer := quic0.NewClientWithPinnedCert(
		addr,
		tlsConf,
		quicConf,
		dialFn,
		pinnedCert,
	)
	return &quic0Impl{addr, dialer}, nil
}

func (impl *quic0Impl) close() {
	log.Debug("Closing quic0 session: Proxy closed.")
	impl.dialer.Close()
}

func (impl *quic0Impl) dialServer(op *ops.Op, ctx context.Context, dialCore dialCoreFn) (net.Conn, error) {
	return dialCore(op, ctx)
}

func (impl *quic0Impl) dialCore(op *ops.Op, ctx context.Context) (net.Conn, error) {
	conn, err := impl.dialer.DialContext(ctx)
	if err != nil {
		log.Debugf("Failed to establish multiplexed quic0 connection: %s", err)
	} else {
		log.Debug("established new multiplexed quic0 connection.")
	}
	return conn, err
}
