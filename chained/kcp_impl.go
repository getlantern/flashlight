// +build !ios

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
	reportDialCore reportDialCoreFn
	addr           string
	dialKCP        func(ctx context.Context, addr string) (net.Conn, error)
}

func newKCPImpl(s *ChainedServerInfo, reportDialCore reportDialCoreFn) (proxyImpl, error) {
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
	return &kcpImpl{
		reportDialCore: reportDialCore,
		// Fix address (comes across as kcp-placeholder)
		addr:    cfg.RemoteAddr,
		dialKCP: dialKCP,
	}, nil
}

func (impl *kcpImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.reportDialCore(op, func() (net.Conn, error) {
		return impl.dialKCP(ctx, impl.addr)
	})
}
