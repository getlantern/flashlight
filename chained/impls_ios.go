//go:build ios

package chained

import (
	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
)

func newKCPImpl(s *config.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, errors.New("KCP is not supported on iOS")
}

func newQUICImpl(name, addr string, s *config.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, errors.New("QUIC is not supported on iOS")
}

func newShadowsocksImpl(name, addr string, s *config.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, errors.New("Shadowsocks is not supported on iOS")
}
