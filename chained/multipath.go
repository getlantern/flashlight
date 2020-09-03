package chained

import (
	"context"
	"fmt"
	"net"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/multipath"
)

type mpDialerAdapter struct {
	impl  proxyImpl
	label string
}

func (d *mpDialerAdapter) DialContext(ctx context.Context) (net.Conn, error) {
	var op *ops.Op
	if value := ctx.Value("op"); value != nil {
		if existing, ok := value.(*ops.Op); ok {
			op = existing
		}
	}
	if op == nil {
		op = ops.Begin("dial_subflow")
		defer op.End()
	}
	return d.impl.dialServer(op, ctx)
}

func (d *mpDialerAdapter) Label() string {
	return d.label
}

type multipathImpl struct {
	nopCloser
	dialer multipath.Dialer
}

func (impl *multipathImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.dialer.DialContext(context.WithValue(ctx, "op", op))
}

func (impl *multipathImpl) FormatStats() []string {
	return impl.dialer.(multipath.Stats).FormatStats()
}

func CreateMPDialer(endpoint string, ss map[string]*ChainedServerInfo, uc common.UserConfig) (balancer.Dialer, error) {
	if len(ss) < 1 {
		return nil, errors.New("no dilers")
	}
	var p *proxy
	var err error
	var dialers []multipath.Dialer
	for name, s := range ss {
		if p == nil {
			// Note: we pass the first server info to newProxy for the attributes shared by all paths
			p, err = newProxy(endpoint, "multipath", "multipath", "multipath", s, uc)
			if err != nil {
				return nil, err
			}
		}
		addr, transport, _, err := extractParams(s)
		if err != nil {
			return nil, err
		}
		impl, err := createImpl(name, addr, transport, s, uc, p.reportDialCore)
		if err != nil {
			log.Errorf("failed to add %v to %v, continuing: %v", s.Addr, name, err)
			continue
		}
		label := fmt.Sprintf("%-38s at %21s", fmt.Sprintf("%s(%s)", name, transport), addr)
		dialers = append(dialers, &mpDialerAdapter{impl, label})
	}
	if len(dialers) == 0 {
		return nil, errors.New("no subflow dialer")
	}
	p.impl = &multipathImpl{dialer: multipath.MPDialer(endpoint, dialers)}
	return p, nil
}
