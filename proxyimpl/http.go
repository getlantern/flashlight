package proxyimpl

import (
	"context"
	"fmt"
	"net"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
)

type httpImpl struct {
	common.NopCloser
	dialCore coreDialer
	addr     string
}

func newHTTPImpl(addr string, dialCore coreDialer) ProxyImpl {
	return &httpImpl{addr: addr, dialCore: dialCore}
}

func (impl *httpImpl) DialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	c, err := impl.dialCore(op, ctx, impl.addr)
	if err != nil {
		return nil, fmt.Errorf("Unable to dial HTTP proxy at %v: %v", impl.addr, err)
	}

	// Run the post-layer 4 dial function if specified
	// if onPostLayer4Dial != nil {
	// 	c, err = onPostLayer4Dial(c.(*net.TCPConn))
	// 	if err != nil {
	// 		return nil, fmt.Errorf("onPostLayer4Dial failed: %v", err)
	// 	}
	// }
	return c, nil
}
