package chained

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
	algeneva "github.com/getlantern/lantern-algeneva"
	"github.com/getlantern/netx"

	"github.com/getlantern/flashlight/v7/ops"
)

type algenevaImpl struct {
	nopCloser
	dialerOps      algeneva.DialerOpts
	addr           string
	reportDialCore reportDialCoreFn
}

func newAlgenevaImpl(addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (*algenevaImpl, error) {
	opts := algeneva.DialerOpts{
		AlgenevaStrategy: ptSetting(pc, "algeneva_strategy"),
	}

	if cert := pc.Cert; cert != "" {
		block, rest := pem.Decode([]byte(pc.Cert))
		if block == nil {
			return nil, errors.New("failed to decode proxy certificate as PEM block")
		}

		if len(rest) > 0 {
			return nil, errors.New("unexpected extra data in proxy certificate PEM")
		}

		if block.Type != "CERTIFICATE" {
			return nil, errors.New("expected certificate in PEM block")
		}

		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM([]byte(cert))
		ip, _, _ := net.SplitHostPort(addr)
		opts.TLSConfig = &tls.Config{
			RootCAs:    certPool,
			ServerName: ip,
		}
	}

	return &algenevaImpl{
		dialerOps:      opts,
		addr:           addr,
		reportDialCore: reportDialCore,
	}, nil
}

func (a *algenevaImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	dialerOps := a.dialerOps
	dialerOps.Dialer = &algenevaDialer{
		a.reportDialCore,
		op,
	}

	conn, err := algeneva.DialContext(ctx, "tcp", a.addr, dialerOps)
	if err != nil {
		return nil, fmt.Errorf("algeneva: %v", err)
	}

	return conn, nil
}

func (*algenevaImpl) isReady() bool {
	return true
}

// algenevaDialer is a algeneva.Dialer wrapper around a reportDialCore. algeneva accepts an optional
// Dialer interface which it will use to dial the server and then wrap the resulting connection.
type algenevaDialer struct {
	reportDialCore reportDialCoreFn
	op             *ops.Op
}

func (d *algenevaDialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

func (d *algenevaDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.reportDialCore(d.op, func() (net.Conn, error) {
		return netx.DialContext(ctx, network, addr)
	})
}
