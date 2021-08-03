package chained

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/url"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/netx"
	"github.com/getlantern/tinywss"
	"github.com/getlantern/tlsdialer/v3"
	tls "github.com/refraction-networking/utls"
)

type wssImpl struct {
	reportDialCore reportDialCoreFn
	addr           string
	dialer         tinywss.Client
}

func newWSSImpl(addr string, s *ChainedServerInfo, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	var rt tinywss.RoundTripHijacker
	var err error

	url := s.ptSetting("url")
	force_http := s.ptSettingBool("force_http")

	if force_http {
		log.Debugf("Using wss http direct")
		rt, err = wssHTTPRoundTripper(s)
		if err != nil {
			return nil, err
		}
	} else {
		log.Debugf("Using wss https direct")
		rt, err = wssHTTPSRoundTripper(s)
		if err != nil {
			return nil, err
		}
	}

	opts := &tinywss.ClientOpts{
		URL:               url,
		RoundTrip:         rt,
		KeepAliveInterval: IdleTimeout / 2,
		KeepAliveTimeout:  IdleTimeout,
		Multiplexed:       s.ptSettingBool("multiplexed"),
		MaxFrameSize:      s.ptSettingInt("max_frame_size"),
		MaxReceiveBuffer:  s.ptSettingInt("max_receive_buffer"),
	}

	client := tinywss.NewClient(opts)
	return &wssImpl{reportDialCore, addr, client}, nil
}

func (impl *wssImpl) close() {
	log.Debug("Closing wss session: Proxy closed.")
	impl.dialer.Close()
}

func (impl *wssImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.reportDialCore(op, func() (net.Conn, error) {
		return impl.dialer.DialContext(ctx)
	})
}

func wssHTTPRoundTripper(s *ChainedServerInfo) (tinywss.RoundTripHijacker, error) {
	return tinywss.NewRoundTripper(func(network, addr string) (net.Conn, error) {
		log.Debugf("tinywss HTTP Roundtripper dialing %v", addr)
		// the configured proxy address is always contacted
		return netx.DialTimeout(network, addr, chainedDialTimeout)
	}), nil
}

func wssHTTPSRoundTripper(s *ChainedServerInfo) (tinywss.RoundTripHijacker, error) {
	serverName := s.TLSServerNameIndicator
	sendServerName := true
	if serverName == "" {
		sendServerName = false
		u, err := url.Parse(s.ptSetting("url"))
		if err != nil {
			return nil, log.Error(errors.Wrap(err).With("addr", s.Addr))
		}
		serverName = u.Hostname()
	}

	forceValidateName := s.ptSetting("force_validate_name")
	helloID := s.clientHelloID()
	certPool := x509.NewCertPool()
	rest := []byte(s.Cert)
	var block *pem.Block
	for {
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, log.Error(errors.Wrap(err).With("addr", s.Addr))
		}
		certPool.AddCert(cert)
	}

	return tinywss.NewRoundTripper(func(network, addr string) (net.Conn, error) {
		tlsConf := &tls.Config{
			CipherSuites: orderedCipherSuitesFromConfig(s),
			ServerName:   serverName,
			RootCAs:      certPool,
			KeyLogWriter: getTLSKeyLogWriter(),
		}

		td := &tlsdialer.Dialer{
			DoDial:            netx.DialTimeout,
			SendServerName:    sendServerName,
			ForceValidateName: forceValidateName,
			Config:            tlsConf,
			ClientHelloID:     helloID,
			Timeout:           chainedDialTimeout,
		}
		// the configured proxy address is always contacted.
		return td.Dial(network, addr)
	}), nil
}
