//go:build !cronet
// +build !cronet

package chained

import (
	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
)

func newCronetImpl(s *config.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, errors.New("Cronet is not supported in this build")
}
