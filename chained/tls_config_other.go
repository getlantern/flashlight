// +build !ios

package chained

import (
	"context"
	"fmt"

	"github.com/getlantern/flashlight/hellocap"
	tls "github.com/refraction-networking/utls"
)

func activelyObtainBrowserHello(configDir string, ctx context.Context) (*tls.ClientHelloSpec, error) {
	const tlsRecordHeaderLen = 5

	activeCaptureHelloCache := helloCacheInConfigDir(configDir, "hello-cache.active-capture")

	helloSpec, err := activeCaptureHelloCache.readAndParse()
	if err == nil {
		return helloSpec, nil
	}
	log.Debugf("failed to read actively obtained hello from cache: %v", err)

	sampleHello, err := hellocap.GetDefaultBrowserHello(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	helloSpec, err = tls.FingerprintClientHello(sampleHello[tlsRecordHeaderLen:])
	if err != nil {
		return nil, fmt.Errorf("failed to fingerprint sample hello: %w", err)
	}
	if err := activeCaptureHelloCache.write(sampleHello); err != nil {
		log.Debugf("failed to write actively obtained hello to cache: %v", err)
	}
	return helloSpec, nil
}
