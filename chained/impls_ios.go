//go:build iosapp
// +build iosapp

package chained

import "github.com/getlantern/errors"

func newKCPImpl(s *apipb.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, errors.New("KCP is not supported on iOS")
}

func newQUICImpl(name, addr string, s *apipb.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, errors.New("QUIC is not supported on iOS")
}

func newShadowsocksImpl(name, addr string, s *apipb.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, errors.New("Shadowsocks is not supported on iOS")
}
