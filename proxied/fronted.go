package proxied

import (
	"net/http"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/fronted"
)

const DefaultMasqueradeTimeout = 5 * time.Minute

// Fronted creates an http.RoundTripper that proxies request using domain
// fronting.
//
// Leave masqueradeTimeout as 0 to use a default value.
func Fronted(masqueradeTimeout time.Duration) http.RoundTripper {
	if masqueradeTimeout == 0 {
		masqueradeTimeout = DefaultMasqueradeTimeout
	}
	return frontedRoundTripper{masqueradeTimeout: masqueradeTimeout}
}

type frontedRoundTripper struct {
	masqueradeTimeout time.Duration
}

// Use a wrapper for fronted.NewFronted to avoid blocking
// `dualFetcher.RoundTrip` when fronted is not yet available, especially when
// the application is starting up
func (f frontedRoundTripper) RoundTrip(
	req *http.Request,
) (*http.Response, error) {
	rt, ok := fronted.NewFronted(f.masqueradeTimeout)
	if !ok {
		return nil, errors.New("Unable to obtain direct fronter")
	}
	changeUserAgent(req)
	return rt.RoundTrip(req)
}
