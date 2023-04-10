package proxyimpl

import (
	"context"
	"net"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
)

type TestImpl struct {
	common.NopCloser
	DialServerFunc func(ctx context.Context) (net.Conn, error)
}

func (impl *TestImpl) DialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.DialServerFunc(ctx)
}
