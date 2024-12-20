package chained

import (
	"context"
	"net"

	"github.com/getlantern/flashlight/v7/ops"
)

type httpImpl struct {
	nopCloser
	dialCore coreDialer
	addr     string
}

var _ proxyImpl = (*httpImpl)(nil)

func newHTTPImpl(addr string, dialCore coreDialer) proxyImpl {
	return &httpImpl{addr: addr, dialCore: dialCore}
}

func (impl *httpImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.dialCore(op, ctx, impl.addr)
}

func (*httpImpl) ready() <-chan error {
	return nil
}
