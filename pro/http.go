package pro

import (
	"net/http"
	"sync/atomic"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/proxied"
)

var (
	httpClient atomic.Value
)

func init() {
	rt := proxied.ChainedThenFrontedWith(common.ProAPIDDFHost, "")
	rtForGet := proxied.ParallelPreferChainedWith(common.ProAPIDDFHost, "")
	httpClient.Store(getHTTPClient(rtForGet, rt))
}

// GetHTTPClient creates a new http.Client that uses domain fronting and direct
// proxies.
func GetHTTPClient() *http.Client {
	return httpClient.Load().(*http.Client)
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
