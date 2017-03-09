package pro

import (
	"net/http"
	"net/http/httptrace"

	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/golog"
)

// GetHTTPClient creates a new http.Client that uses domain fronting and direct
// proxies.
func GetHTTPClient() *http.Client {
	rt := proxied.ChainedThenFronted()
	rtForGet := proxied.ParallelPreferChained()
	return getHTTPClient(rtForGet, rt)
}

func getHTTPClient(getRt, otherRt http.RoundTripper) *http.Client {
	log := golog.LoggerFor("flashlight.pro.http")
	return &http.Client{
		Transport: proxied.AsRoundTripper(func(req *http.Request) (*http.Response, error) {
			trace := &httptrace.ClientTrace{
				DNSDone: func(info httptrace.DNSDoneInfo) {
					log.Debugf("DNS Info: %+v\n", info)
				},
				GotConn: func(info httptrace.GotConnInfo) {
					log.Debugf("Got Conn: %+v\n", info)
				},
			}
			req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
			frontedURL := *req.URL
			frontedURL.Host = proAPIDDFHost
			proxied.PrepareForFronting(req, frontedURL.String())
			if req.Method == "GET" || req.Method == "HEAD" {
				return getRt.RoundTrip(req)
			}
			return otherRt.RoundTrip(req)
		}),
	}
}
