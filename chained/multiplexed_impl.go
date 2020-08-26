package chained

import (
	"context"
	"net"

	"github.com/getlantern/cmux"

	"github.com/getlantern/flashlight/ops"
)

type multiplexedImpl struct {
	proxyImpl
	multiplexedDial cmux.DialFN
}

func multiplexed(wrapped proxyImpl, name string, poolSize int, dialCore dialCoreFn) proxyImpl {
	log.Debugf("Enabling multiplexing for %v", name)
	if poolSize < 1 {
		poolSize = defaultMultiplexedPhysicalConns
	}
	multiplexedDial := cmux.Dialer(&cmux.DialerOpts{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			op := ops.Begin("dial_multiplexed")
			defer op.End()
			return wrapped.dialServer(op, ctx, dialCore)
		},
		KeepAliveInterval: IdleTimeout / 2,
		KeepAliveTimeout:  IdleTimeout,
		PoolSize:          poolSize,
	})
	return &multiplexedImpl{wrapped, multiplexedDial}
}

func (impl *multiplexedImpl) dialServer(op *ops.Op, ctx context.Context, dialCore dialCoreFn) (net.Conn, error) {
	return impl.multiplexedDial(ctx, "", "")
}
