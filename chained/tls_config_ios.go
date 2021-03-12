// +build ios

package chained

import (
	"context"
	"fmt"
)

func activelyObtainBrowserHello(ctx context.Context, configDir string) (*hello, error) {
	return nil, fmt.Errorf("activelyObtainBrowserHello not supported on iOS")
}
