// +build android darwin,!amd64

package chained

import "errors"

func enableUTP(p *proxy, s *ChainedServerInfo) error {
	return errors.New("UTP is not supported on Android or iOS")
}
