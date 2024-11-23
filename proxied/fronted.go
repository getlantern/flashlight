package proxied

import (
	"fmt"
	"net/http"
	"time"

	"github.com/getlantern/flashlight/v7/ops"
	"github.com/getlantern/fronted"
)

const DefaultMasqueradeTimeout = 5 * time.Minute

// Fronted creates an http.RoundTripper that proxies request using domain
// fronting.
//
// Leave masqueradeTimeout as 0 to use a default value.
func Fronted(opName string, masqueradeTimeout time.Duration,
	fronted fronted.Fronting) http.RoundTripper {
	if masqueradeTimeout == 0 {
		masqueradeTimeout = DefaultMasqueradeTimeout
	}
	return frontedRoundTripper{
		masqueradeTimeout: masqueradeTimeout,
		opName:            opName,
		fronted:           fronted,
	}
}

type frontedRoundTripper struct {
	masqueradeTimeout time.Duration
	opName            string
	fronted           fronted.Fronting
}

// Use a wrapper for fronted.NewDirect to avoid blocking
// `dualFetcher.RoundTrip` when fronted is not yet available, especially when
// the application is starting up
func (f frontedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.opName != "" {
		op := ops.Begin(f.opName)
		defer op.End()
	}
	rt, err := f.fronted.NewRoundTripper(f.masqueradeTimeout)
	if err != nil {
		return nil, fmt.Errorf("unable to obtain direct fronter %v", err)
	}
	changeUserAgent(req)
	return rt.RoundTrip(req)
}
