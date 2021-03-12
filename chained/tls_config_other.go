// +build !ios

package chained

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/getlantern/flashlight/hellocap"
	tls "github.com/refraction-networking/utls"
)

const helloCacheFilename = "hello-cache.active-capture"

// TODO: look at why ios is separated out
func activelyObtainBrowserHello(ctx context.Context, configDir string) (*helloSpec, error) {
	const tlsRecordHeaderLen = 5

	helloCacheFile := filepath.Join(configDir, helloCacheFilename)
	sample, err := ioutil.ReadFile(helloCacheFile)
	if err == nil {
		return &helloSpec{tls.HelloCustom, sample}, nil
	}
	log.Debugf("failed to read actively obtained hello from cache: %v", err)

	sample, err = hellocap.GetDefaultBrowserHello(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	if err := ioutil.WriteFile(helloCacheFile, sample, 0644); err != nil {
		log.Debugf("failed to write actively obtained hello to cache: %v", err)
	}
	return &helloSpec{tls.HelloCustom, sample}, nil
}
