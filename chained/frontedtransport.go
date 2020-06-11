package chained

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight/flfronting"
	"github.com/getlantern/fronted"
	"github.com/getlantern/netx"
	"github.com/getlantern/tinywss"
	"github.com/getlantern/tlsdialer"
	tls "github.com/refraction-networking/utls"
)

type frontedTransport struct {
	rt eventual.Value // http.RoundTripper
}

func newFrontedTransport(opts fronted.RoundTripperOptions, msgIfErr string) frontedTransport {
	ft := frontedTransport{eventual.NewValue()}
	go func() {
		rt, err := flfronting.NewRoundTripper(context.Background(), opts)
		if err != nil {
			log.Errorf("%s: %v", msgIfErr, err)
			return
		}
		ft.rt.Set(rt)
	}()
	return ft
}

func (ft frontedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt, err := getEventualContext(req.Context(), ft.rt)
	if err != nil {
		return nil, fmt.Errorf("unable to obtain fronted roundtripper: %w", err)
	}
	return rt.(http.RoundTripper).RoundTrip(req)
}

func wssHTTPRoundTripper(p *proxy, s *ChainedServerInfo) (tinywss.RoundTripHijacker, error) {
	return tinywss.NewRoundTripper(func(network, addr string) (net.Conn, error) {
		log.Debugf("tinywss HTTP Roundtripper dialing %v", addr)
		// the configured proxy address is always contacted
		return netx.DialTimeout(network, p.addr, chainedDialTimeout)
	}), nil
}

func wssHTTPSRoundTripper(p *proxy, s *ChainedServerInfo) (tinywss.RoundTripHijacker, error) {

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
		return td.Dial(network, p.addr)
	}), nil
}

func getEventualContext(ctx context.Context, v eventual.Value) (interface{}, error) {
	// The default timeout is large enough that it shouldn't supercede the context timeout, but
	// short enough that we'll eventually clean up launched routines.
	const defaultTimeout = time.Hour

	timeout := defaultTimeout
	if deadline, ok := ctx.Deadline(); ok {
		// Add 10s to the context timeout to ensure ctx.Done() closes before v.Get times out.
		timeout = time.Until(deadline) + 10*time.Second
	}
	resultCh := make(chan interface{})
	go func() {
		result, _ := v.Get(timeout)
		resultCh <- result
	}()
	select {
	case r := <-resultCh:
		return r, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
