// +build !ios

package chained

import (
	"context"
	gtls "crypto/tls"
	"encoding/base64"
	"net"
	"time"

	"github.com/mitchellh/mapstructure"

	"github.com/getlantern/errors"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/kcpwrapper"
	"github.com/getlantern/keyman"
	"github.com/getlantern/mtime"
	"github.com/getlantern/quic0"
	"github.com/getlantern/quicwrapper"
)

func enableKCP(p *proxy, s *ChainedServerInfo) error {
	var cfg KCPConfig
	err := mapstructure.Decode(s.KCPSettings, &cfg)
	if err != nil {
		return log.Errorf("Could not decode kcp transport settings?: %v", err)
	}
	p.kcpConfig = &cfg

	// Fix address (comes across as kcp-placeholder)
	p.addr = cfg.RemoteAddr
	// KCP consumes a lot of bandwidth, so we want to bias against using it unless
	// everything else is blocked. However, we prefer it to domain-fronting. We
	// only default the bias if none was configured.
	if p.bias == 0 {
		p.bias = -1
	}

	addIdleTiming := func(conn net.Conn) net.Conn {
		log.Debug("Wrapping KCP with idletiming")
		return idletiming.Conn(conn, IdleTimeout*2, func() {
			log.Debug("KCP connection idled")
		})
	}
	dialKCP := kcpwrapper.Dialer(&cfg.DialerConfig, addIdleTiming)

	p.doDialCore = func(ctx context.Context) (net.Conn, time.Duration, error) {
		elapsed := mtime.Stopwatch()

		conn, err := dialKCP(ctx, "tcp", p.addr)
		delta := elapsed()
		return conn, delta, err
	}

	return nil
}

func enableQUIC0(p *proxy, s *ChainedServerInfo) error {
	addr := s.Addr
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
		return log.Error(errors.Wrap(err).With("addr", s.Addr))
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
	// when the proxy closes, close the dialer
	go func() {
		<-p.closeCh
		log.Debug("Closing quic0 session: Proxy closed.")
		dialer.Close()
	}()

	p.doDialCore = func(ctx context.Context) (net.Conn, time.Duration, error) {
		elapsed := mtime.Stopwatch()
		conn, err := dialer.DialContext(ctx)
		if err != nil {
			log.Debugf("Failed to establish multiplexed quic0 connection: %s", err)
		} else {
			log.Debug("established new multiplexed quic0 connection.")
		}
		delta := elapsed()
		return conn, delta, err
	}

	return nil
}

func enableQUIC(p *proxy, s *ChainedServerInfo) error {
	addr := s.Addr
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
		return log.Error(errors.Wrap(err).With("addr", s.Addr))
	}
	pinnedCert := cert.X509()

	dialFn := quicwrapper.DialWithNetx

	if s.PluggableTransport == "oquic" {
		oquicKeyStr := s.ptSetting("oquic_key")
		if oquicKeyStr == "" {
			return log.Error("Missing oquic_key for oquic transport")
		}
		oquicKey, err := base64.StdEncoding.DecodeString(oquicKeyStr)
		if err != nil {
			return log.Error(errors.Wrap(err).With("oquic_key", oquicKeyStr))
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
			return log.Errorf("Unable to create oquic dialer: %v", err)
		}
	}

	dialer := quicwrapper.NewClientWithPinnedCert(
		addr,
		tlsConf,
		quicConf,
		dialFn,
		pinnedCert,
	)
	// when the proxy closes, close the dialer
	go func() {
		<-p.closeCh
		log.Debug("Closing quic session: Proxy closed.")
		dialer.Close()
	}()

	p.doDialCore = func(ctx context.Context) (net.Conn, time.Duration, error) {
		elapsed := mtime.Stopwatch()
		conn, err := dialer.DialContext(ctx)
		if err != nil {
			log.Debugf("Failed to establish multiplexed connection: %s", err)
		} else {
			log.Debug("established new multiplexed quic connection.")
		}
		delta := elapsed()
		return conn, delta, err
	}

	return nil
}
