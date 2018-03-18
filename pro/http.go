package pro

import (
	"net/http"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/proxied"
)

var (
	httpClient = getHTTPClient(proxied.ParallelPreferChainedWith(common.ProAPIDDFHost, ""),
		proxied.ChainedThenFrontedWith(common.ProAPIDDFHost, ""))
)

// GetHTTPClient creates a new http.Client that uses domain fronting and direct
// proxies.
func GetHTTPClient() *http.Client {
	return httpClient
}

func getHTTPClient(getRt, otherRt http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: proxied.AsRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == "GET" || req.Method == "HEAD" {
				return getRt.RoundTrip(req)
			}
			return otherRt.RoundTrip(req)
		}),
		// Not follow redirects
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
