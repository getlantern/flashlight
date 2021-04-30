// +build !iosapp

package chained

import (
	"context"
	gtls "crypto/tls"
	"encoding/base64"
	"net"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/keyman"
	"github.com/getlantern/quicwrapper"
)

type quicImpl struct {
	reportDialCore reportDialCoreFn
	addr           string
	dialer         *quicwrapper.Client
}

func newQUICImpl(name, addr string, s *ChainedServerInfo, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	tlsConf := &gtls.Config{
		ServerName:         s.TLSServerNameIndicator,
		InsecureSkipVerify: true,
		KeyLogWriter:       getTLSKeyLogWriter(),
	}

	quicConf := &quicwrapper.Config{
		MaxIncomingStreams: -1,
		KeepAlive:          true,
	}

	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", addr))
	}
	pinnedCert := cert.X509()

	dialFn := quicwrapper.DialWithNetx

	if s.PluggableTransport == "oquic" {
		oquicKeyStr := s.ptSetting("oquic_key")
		if oquicKeyStr == "" {
			return nil, log.Error("Missing oquic_key for oquic transport")
		}
		oquicKey, err := base64.StdEncoding.DecodeString(oquicKeyStr)
		if err != nil {
			return nil, log.Error(errors.Wrap(err).With("oquic_key", oquicKeyStr))
		}
		oquicConfig := quicwrapper.DefaultOQuicConfig(oquicKey)
		if cipher := s.ptSetting("oquic_cipher"); cipher != "" {
			oquicConfig.Cipher = cipher
		}
		if s.ptSetting("oquic_aggressive_padding") != "" {
			oquicConfig.AggressivePadding = int64(s.ptSettingInt("oquic_aggressive_padding"))
		}
		if s.ptSetting("oquic_max_padding_hint") != "" {
			oquicConfig.MaxPaddingHint = uint8(s.ptSettingInt("oquic_max_padding_hint"))
		}
		if s.ptSetting("oquic_min_padded") != "" {
			oquicConfig.MinPadded = s.ptSettingInt("oquic_min_padded")
		}

		dialFn, err = quicwrapper.NewOQuicDialerWithUDPDialer(quicwrapper.DialUDPNetx, oquicConfig)
		if err != nil {
			return nil, log.Errorf("Unable to create oquic dialer: %v", err)
		}
	}

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
