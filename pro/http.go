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

// PrepareForFronting prepares the given request to be used with domain-
// fronting.
func PrepareForFronting(req *http.Request) {
	log := golog.LoggerFor("flashlight.pro.http")
	if req == nil {
		return
	}
	frontedURL := *req.URL
	frontedURL.Host = proAPIDDFHost
	proxied.PrepareForFronting(req, frontedURL.String())
	trace := &httptrace.ClientTrace{
		DNSDone: func(info httptrace.DNSDoneInfo) {
			log.Debugf("DNS Info: %+v\n", info)
		},
		GotConn: func(info httptrace.GotConnInfo) {
			log.Debugf("Got Conn: %+v\n", info)
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
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
