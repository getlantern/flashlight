package pro

import (
	"net/http"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/proxied"
)

// GetHTTPClient creates a new http.Client that uses domain fronting and direct
// proxies.
func GetHTTPClient() *http.Client {
	rt := proxied.ChainedThenFrontedWith(common.ProAPIDDFHost, "")
	// Respond sooner if chained proxy is blocked, but only for idempotent requests (GETs)
	rtForGet := proxied.ParallelPreferChainedWith(common.ProAPIDDFHost, "")
	return getHTTPClient(rtForGet, rt)
}

func getHTTPClient(getRt, otherRt http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: proxied.AsRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == "GET" || req.Method == "HEAD" {
				return getRt.RoundTrip(req)
			}
			return otherRt.RoundTrip(req)
		}),
	}
}
