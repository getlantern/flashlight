package chained

import (
	"context"
	"net"

	"github.com/getlantern/common/config"
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
	strategy := ptSetting(pc, "algeneva_strategy")

	ops := algeneva.DialerOpts{
		AlgenevaStrategy: strategy,
	}
	return &algenevaImpl{
		dialerOps:      ops,
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

	return algeneva.DialContext(ctx, "tcp", a.addr, dialerOps)
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
