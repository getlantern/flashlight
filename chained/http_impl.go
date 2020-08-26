package chained

import (
	"context"
	"net"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/netx"
)

type httpImpl struct {
	nopCloser
	reportDialCore reportDialCoreFn
	addr           string
}

func (impl *httpImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.reportDialCore(op, func() (net.Conn, error) {
		return netx.DialContext(ctx, "tcp", impl.addr)
	})
}
