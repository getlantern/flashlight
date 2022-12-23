//go:build ios

package chained

import (
	"fmt"

	"github.com/getlantern/common/config"
)

func newShadowsocksImpl(name, addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, fmt.Errorf("newShadowsocksImpl not supported on iOS")
}
