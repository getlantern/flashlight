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

var fronter fronted.Fronted

func InitFronted() fronted.Fronted {
	var cacheFile string
	dir, err := os.UserConfigDir()
	if err != nil {
		_ = log.Errorf("Unable to get user config dir: %v", err)
	} else {
		cacheFile = filepath.Join(dir, common.DefaultAppName, "fronted_cache.json")
	}
	return fronted.NewFronted(cacheFile)
}

// Fronted creates a http.RoundTripper that proxies request using domain
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
