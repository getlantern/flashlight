package proxyimpl

import (
	"context"
	"fmt"
	"net"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/multipath"
)

type mpDialerAdapter struct {
	impl  ProxyImpl
	label string
}

func (d *mpDialerAdapter) DialContext(ctx context.Context) (net.Conn, error) {
	var op *ops.Op
	if value := ctx.Value("op"); value != nil {
		if existing, ok := value.(*ops.Op); ok {
			op = existing.Begin("dial_subflow")
		}
	}
	if op == nil {
		op = ops.Begin("dial_subflow")
	}
	defer op.End()
	return d.impl.DialServer(op, ctx)
}

func (d *mpDialerAdapter) Label() string {
	return d.label
}

type MultipathImpl struct {
	common.NopCloser
	dialer multipath.Dialer
}

func (impl *MultipathImpl) DialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.dialer.DialContext(context.WithValue(ctx, "op", op))
}

func (impl *MultipathImpl) FormatStats() []string {
	return impl.dialer.(multipath.Stats).FormatStats()
}

func NewMultipathProxyImpl(
	configDir, endpoint string,
	ss map[string]*config.ProxyConfig,
	uc common.UserConfig,
	extractProxyConfigParamsFunc func(*config.ProxyConfig) (addr string, transport string, network string, err error),
	defaultReportDialCore ReportDialCoreFn,
) (ProxyImpl, error) {
	if len(ss) < 1 {
		return nil, errors.New("no dialers")
	}
	var dialers []multipath.Dialer
	for name, s := range ss {
		addr, transport, _, err := extractProxyConfigParamsFunc(s)
		if err != nil {
			return nil, err
		}
		impl, err := New(configDir, name, addr, transport, s, uc, defaultReportDialCore)
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
	return &MultipathImpl{dialer: multipath.NewDialer(endpoint, dialers)}, nil
}

func GroupByMultipathEndpoint(proxies map[string]*config.ProxyConfig) map[string]map[string]*config.ProxyConfig {
	groups := make(map[string]map[string]*config.ProxyConfig)
	for name, s := range proxies {
		group, exists := groups[s.MultipathEndpoint]
		if !exists {
			group = make(map[string]*config.ProxyConfig)
			groups[s.MultipathEndpoint] = group
		}
		group[name] = s
	}
	return groups
}
