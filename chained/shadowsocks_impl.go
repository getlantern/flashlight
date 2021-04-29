// +build !ios

package chained

import (
	"context"
	"fmt"
	"net"
	"strconv"

	shadowsocks "github.com/getlantern/lantern-shadowsocks/client"

	"github.com/getlantern/flashlight/ops"
)

type shadowsocksImpl struct {
	reportDialCore reportDialCoreFn
	client         shadowsocks.Client
	upstream       string
}

func newShadowsocksImpl(name, addr string, s *ChainedServerInfo, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	secret := s.ptSetting("shadowsocks_secret")
	cipher := s.ptSetting("shadowsocks_cipher")
	upstream := s.ptSetting("shadowsocks_upstream")

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("unable to parse port in address %v: %v", addr, err)
	}
	client, err := shadowsocks.NewClient(host, port, secret, cipher)
	if err != nil {
		return nil, err
	}
	return &shadowsocksImpl{
		reportDialCore: reportDialCore,
		client:         client,
		upstream:       upstream,
	}, nil
}

func (impl *shadowsocksImpl) close() {
}

func (impl *shadowsocksImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.reportDialCore(op, func() (net.Conn, error) {
		return impl.client.DialTCP(nil, impl.upstream)
	})
}
