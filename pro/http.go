package pro

import (
	"net/http"

	"github.com/getlantern/flashlight/proxied"
)

var (
	httpClient *http.Client
)

func init() {
	httpClient = getHTTPClient(
		&proxied.MaybeProxiedFlowRoundTripper{
			Default: proxied.ParallelForIdempotent(),
			Flow: proxied.NewProxiedFlow(
				&proxied.ProxiedFlowOptions{
					ParallelMethods: []string{http.MethodGet, http.MethodHead},
				}).Add(proxied.FlowComponentID_Chained, true).
				Add(proxied.FlowComponentID_Fronted, false).
				Add(proxied.FlowComponentID_Broflake, false)})
}

// GetHTTPClient creates a new http.Client that uses domain fronting and direct
// proxies.
func GetHTTPClient() *http.Client {
	return httpClient
}

func getHTTPClient(rt http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: rt,
		// Don't follow redirects
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
