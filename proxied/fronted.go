package proxied

import (
	"net/http"

	"github.com/getlantern/flashlight/v7/ops"
)

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
