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

func newHTTPImpl(addr string, dialCore coreDialer) ProxyImpl {
	return &httpImpl{addr: addr, dialCore: dialCore}
}
func (impl *httpImpl) DialServer(op *ops.Op, ctx context.Context, prefix []byte) (net.Conn, error) {
	return impl.dialCore(op, ctx, impl.addr)
}
