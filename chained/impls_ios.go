// +build ios

package chained

import "github.com/getlantern/errors"

func newKCPImpl(s *ChainedServerInfo, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, errors.New("KCP is not supported on iOS")
}

func newQUIC0Impl(name, addr string, s *ChainedServerInfo, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, errors.New("QUIC0 is not supported on iOS")
}

func newQUICImpl(name, addr string, s *ChainedServerInfo, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, errors.New("QUIC is not supported on iOS")
}
