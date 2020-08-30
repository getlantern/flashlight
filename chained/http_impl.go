package chained

import (
	"context"
	"net"

	"github.com/getlantern/flashlight/ops"
)

type httpImpl struct {
	nopCloser
	dialCore coreDialer
	addr     string
}

func newHTTPImpl(addr string, dialCore coreDialer) proxyImpl {
	return &httpImpl{addr: addr, dialCore: dialCore}
}
func (impl *httpImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.dialCore(op, ctx, impl.addr)
}
