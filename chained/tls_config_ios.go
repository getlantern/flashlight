// +build iosapp

package chained

import (
	"context"
	"fmt"
)

func activelyObtainBrowserHello(ctx context.Context, configDir string) (*helloSpec, error) {
	return nil, fmt.Errorf("activelyObtainBrowserHello not supported on iOS")
}
