package pro

import (
	"net/http"
	"time"

	proxied "github.com/getlantern/flashlight/v7/proxied"
)

var (
	httpClient = getHTTPClient(proxied.ParallelForIdempotent())
)

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
		Timeout: 30 * time.Second,
	}
}
