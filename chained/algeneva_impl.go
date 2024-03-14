package chained

import (
	"context"
	"net"

	"github.com/getlantern/common/config"
	algeneva "github.com/getlantern/lantern-algeneva"

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
	return a.reportDialCore(op, func() (net.Conn, error) {
		return algeneva.DialContext(ctx, "tcp", a.addr, a.dialerOps)
	})
}
