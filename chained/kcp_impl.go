//go:build !ios

package chained

import (
	"context"
	"net"

	"github.com/getlantern/flashlight/v7/ops"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
	"github.com/getlantern/kcpwrapper"
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

func newKCPImpl(pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, errors.New("KCP not supported")
}

func (impl *kcpImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.reportDialCore(op, func() (net.Conn, error) {
		return impl.dialKCP(ctx, impl.addr)
	})
}
