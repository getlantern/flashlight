// +build ios

package chained

import "errors"

func enableKCP(p *proxy, s *ChainedServerInfo) error {
	return errors.New("KCP is not supported on iOS")
}

func enableQUIC0(p *proxy, s *ChainedServerInfo) error {
	return errors.New("QUIC0 is not supported on iOS")
}

func enableQUIC(p *proxy, s *ChainedServerInfo) error {
	return errors.New("QUIC is not supported on iOS")
}
