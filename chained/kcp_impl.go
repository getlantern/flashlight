package chained

import (
	"context"
	"net"

	"github.com/mitchellh/mapstructure"

	"github.com/getlantern/idletiming"
	"github.com/getlantern/kcpwrapper"

	"github.com/getlantern/flashlight/ops"
)

// KCPConfig adapts kcpwrapper.DialerConfig to the currently deployed
// configurations in order to provide backward-compatibility.
type KCPConfig struct {
	kcpwrapper.DialerConfig `mapstructure:",squash"`
	RemoteAddr              string `json:"remoteaddr"`
}

type kcpImpl struct {
	nopCloser
	addr    string
	dialKCP func(ctx context.Context, network, addr string) (net.Conn, error)
}

func newKCPImpl(s *ChainedServerInfo) (proxyImpl, error) {
	var cfg KCPConfig
	err := mapstructure.Decode(s.KCPSettings, &cfg)
	if err != nil {
		return nil, log.Errorf("Could not decode kcp transport settings?: %v", err)
	}
	addIdleTiming := func(conn net.Conn) net.Conn {
		log.Debug("Wrapping KCP with idletiming")
		return idletiming.Conn(conn, IdleTimeout*2, func() {
			log.Debug("KCP connection idled")
		})
	}
	dialKCP := kcpwrapper.Dialer(&cfg.DialerConfig, addIdleTiming)
	log.Errorf("********dialKCP==%+v", dialKCP)
	return &kcpImpl{
		// Fix address (comes across as kcp-placeholder)
		addr:    cfg.RemoteAddr,
		dialKCP: dialKCP,
	}, nil
}

func (impl *kcpImpl) dialServer(op *ops.Op, ctx context.Context, dialCore dialCoreFn) (net.Conn, error) {
	return dialCore(op, ctx)
}

func (impl *kcpImpl) dialCore(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.dialKCP(ctx, "tcp", impl.addr)
}
