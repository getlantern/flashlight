//go:build ios

package chained

import (
	"context"
	"fmt"
)

func cachedHello(configDir string) (*helloSpec, error) {
	return nil, fmt.Errorf("cachedHello not supported on iOS")
}

func ActivelyObtainBrowserHello(ctx context.Context, configDir string) (*helloSpec, error) {
	return nil, fmt.Errorf("activelyObtainBrowserHello not supported on iOS")
}
