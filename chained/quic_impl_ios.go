//go:build ios

package chained

import (
	"fmt"

	"github.com/getlantern/common/config"
)

func newQUICImpl(name, addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	return nil, fmt.Errorf("newQUICImpl not supported on iOS")
}
