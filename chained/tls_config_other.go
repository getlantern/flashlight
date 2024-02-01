//go:build !ios

package chained

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tls "github.com/refraction-networking/utls"

	"github.com/getlantern/flashlight/v7/hellocap"
)

const helloCacheFilename = "hello-cache.active-capture"

func cachedHello(configDir string) (*helloSpec, error) {
	helloCacheFile := filepath.Join(configDir, helloCacheFilename)
	sample, err := os.ReadFile(helloCacheFile)
	if err == nil {
		return &helloSpec{tls.HelloCustom, sample}, nil
	}
	return nil, fmt.Errorf("%w", err)
}

// ActivelyObtainBrowserHello obtains a sample TLS ClientHello via listening
// for local traffic.
func ActivelyObtainBrowserHello(ctx context.Context, configDir string) (*helloSpec, error) {
	sample, err := hellocap.GetDefaultBrowserHello(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	helloCacheFile := filepath.Join(configDir, helloCacheFilename)
	if err := os.WriteFile(helloCacheFile, sample, 0644); err != nil {
		log.Debugf("failed to write actively obtained hello to cache: %v", err)
	} else {
		log.Debugf("wrote actively obtained hello to cache")
	}
	return &helloSpec{tls.HelloCustom, sample}, nil
}
