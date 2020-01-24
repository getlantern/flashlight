// +build android ios linux

package chained

import "errors"

func enableUTP(p *proxy, s *ChainedServerInfo) error {
	return errors.New("UTP is not supported on Android, iOS or Linux")
}
