//go:build !iosapp
// +build !iosapp

package chained

import (
	"context"
	"net"

	"github.com/getlantern/errors"
	"github.com/getlantern/kcpwrapper"

	"github.com/getlantern/flashlight/api/apipb"
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

func newKCPImpl(pc *apipb.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, errors.New("KCP not supported")
}

func (impl *kcpImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.reportDialCore(op, func() (net.Conn, error) {
		return impl.dialKCP(ctx, impl.addr)
	})
}
