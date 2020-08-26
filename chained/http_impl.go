package chained

import (
	"context"
	"net"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/netx"
)

type httpImpl struct {
	nopCloser
	addr string
}

func (impl *httpImpl) dialServer(op *ops.Op, ctx context.Context, dialCore dialCoreFn) (net.Conn, error) {
	return dialCore(op, ctx)
}

func (impl *httpImpl) dialCore(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return netx.DialTimeout("tcp", impl.addr, timeoutFor(ctx))
}
