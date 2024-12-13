package proxied

import (
	"crypto/x509"
	"net/http"
	"os"
	"path/filepath"

	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/ops"
	"github.com/getlantern/fronted"
)

var fronter fronted.Fronted = newFronted()

func newFronted() fronted.Fronted {
	var cacheFile string
	dir, err := os.UserConfigDir()
	if err != nil {
		log.Errorf("Unable to get user config dir: %v", err)
	} else {
		cacheFile = filepath.Join(dir, common.DefaultAppName, "fronted_cache.json")
		if err := os.MkdirAll(filepath.Dir(cacheFile), 0755); err != nil {
			log.Errorf("Failed to create directory: %v", err)
		}
		_, error := os.Stat(cacheFile)
		if error != nil {
			log.Debugf("Cache file  not found at: %v", cacheFile)
			_, err := os.Create(cacheFile)
			log.Debugf("Cache file created at: %v", cacheFile)
			if err != nil {
				log.Errorf("Unable to create cache file: %v", err)
			}
		}
	}
	return fronted.NewFronted(cacheFile)
}

// Fronted creates an http.RoundTripper that proxies request using domain
// fronting.
func Fronted(opName string) http.RoundTripper {
	return frontedRoundTripper{
		opName: opName,
	}
}

type frontedRoundTripper struct {
	opName string
}

// Use a wrapper for fronted.NewDirect to avoid blocking
// `dualFetcher.RoundTrip` when fronted is not yet available, especially when
// the application is starting up
func (f frontedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.opName != "" {
		op := ops.Begin(f.opName)
		defer op.End()
	}
	return fronter.RoundTrip(req)
}

// OnNewFronts updates the fronted configuration with the new fronted providers.
func OnNewFronts(pool *x509.CertPool, providers map[string]*fronted.Provider) {
	fronter.OnNewFronts(pool, providers)
}
