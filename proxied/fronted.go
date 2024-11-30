package proxied

import (
	"net/http"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/v7/ops"
)

const DefaultMasqueradeTimeout = 5 * time.Minute

// Fronted creates an http.RoundTripper that proxies request using domain
// fronting.
//
// Leave masqueradeTimeout as 0 to use a default value.
func Fronted(opName string, masqueradeTimeout time.Duration) http.RoundTripper {
	if masqueradeTimeout == 0 {
		masqueradeTimeout = DefaultMasqueradeTimeout
	}
	return frontedRoundTripper{
		masqueradeTimeout: masqueradeTimeout,
		opName:            opName,
	}
}

type frontedRoundTripper struct {
	masqueradeTimeout time.Duration
	opName            string
}

// Use a wrapper for fronted.NewDirect to avoid blocking
// `dualFetcher.RoundTrip` when fronted is not yet available, especially when
// the application is starting up
func (f frontedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.opName != "" {
		op := ops.Begin(f.opName)
		defer op.End()
	}
	rt, err := fronter.NewRoundTripper(f.masqueradeTimeout)
	if err != nil {
		return nil, errors.New("Unable to obtain direct fronter")
	}
	changeUserAgent(req)
	return rt.RoundTrip(req)
}
