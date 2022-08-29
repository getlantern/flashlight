//go:build !iosapp
// +build !iosapp

package chained

import (
	"context"
	gtls "crypto/tls"
	"net"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/keyman"
	"github.com/getlantern/lantern-cloud/cmd/api/apipb"
	"github.com/getlantern/quicwrapper"
)

type quicImpl struct {
	reportDialCore reportDialCoreFn
	addr           string
	dialer         *quicwrapper.Client
}

func newQUICImpl(name, addr string, pc *apipb.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	tlsConf := &gtls.Config{
		ServerName:         pc.TLSServerNameIndicator,
		InsecureSkipVerify: true,
		KeyLogWriter:       getTLSKeyLogWriter(),
	}

	disablePathMTUDiscovery := true
	if ptSettingBool(pc, "path_mtu_discovery") == true {
		disablePathMTUDiscovery = false
	}

	quicConf := &quicwrapper.Config{
		MaxIncomingStreams: -1,
		// This interface was changed in quic-go v0.28.0. This is what I think it was for "true"
		// before that version. It's definitely not more than 20s.
		KeepAlivePeriod:         15 * time.Second,
		DisablePathMTUDiscovery: disablePathMTUDiscovery,
	}

	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(pc.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", addr))
	}
	pinnedCert := cert.X509()

	dialFn := quicwrapper.DialWithNetx
	dialer := quicwrapper.NewClientWithPinnedCert(
		addr,
		tlsConf,
		quicConf,
		dialFn,
		pinnedCert,
	)
	return &quicImpl{reportDialCore, addr, dialer}, nil
}

func (impl *quicImpl) close() {
	log.Debug("Closing quic session: Proxy closed.")
	impl.dialer.Close()
}

func (impl *quicImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.reportDialCore(op, func() (net.Conn, error) {
		conn, err := impl.dialer.DialContext(ctx)
		if err != nil {
			log.Debugf("Failed to establish multiplexed connection: %s", err)
		} else {
			log.Debug("established new multiplexed quic connection.")
		}
		return conn, err
	})
}
