// +build ios

package chained

import (
	"context"
	"fmt"

	tls "github.com/refraction-networking/utls"
)

func activelyObtainBrowserHello(configDir string, ctx context.Context) (*tls.ClientHelloSpec, error) {
	return nil, fmt.Errorf("activelyObtainBrowserHello not supported on iOS")
}
