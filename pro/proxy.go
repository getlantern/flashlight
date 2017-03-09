package pro

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/golog"
)

var (
	log        = golog.LoggerFor("flashlight.pro")
	httpClient = &http.Client{Transport: proxied.ChainedThenFronted()}
	// Respond sooner if chained proxy is blocked, but only for idempotent requests (GETs)
	httpClientForGET = &http.Client{Transport: proxied.ParallelPreferChained()}
)

type proxyTransport struct {
	// Satisfies http.RoundTripper
}

func (pt *proxyTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	origin := req.Header.Get("Origin")
	if req.Method == "OPTIONS" {
		// No need to proxy the OPTIONS request.
		resp = &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Connection":                   {"keep-alive"},
				"Access-Control-Allow-Methods": {"GET, POST"},
				"Access-Control-Allow-Headers": {req.Header.Get("Access-Control-Request-Headers")},
				"Via": {"Lantern Client"},
			},
			Body: ioutil.NopCloser(strings.NewReader("preflight complete")),
		}
	} else {
		// Workaround for https://github.com/getlantern/pro-server/issues/192
		req.Header.Del("Origin")
		if req.Method == "GET" {
			resp, err = httpClientForGET.Do(req)
		} else {
			resp, err = httpClient.Do(req)
		}
		if err != nil {
			log.Errorf("Could not issue HTTP request? %v", err)
			return
		}
	}
	resp.Header.Set("Access-Control-Allow-Origin", origin)
	return
}

func GetHTTPClient() *http.Client {
	rt := proxied.ChainedThenFronted()
	rtForGet := proxied.ParallelPreferChained()
	return &http.Client{
		Transport: proxied.AsRoundTripper(func(req *http.Request) (*http.Response, error) {
			frontedURL := *req.URL
			frontedURL.Host = proAPIDDFHost
			proxied.PrepareForFronting(req, frontedURL.String())
			if req.Method == "GET" {
				return rtForGet.RoundTrip(req)
			} else {
				return rt.RoundTrip(req)
			}
		}),
	}
}

// APIHandler returns an HTTP handler that specifically looks for and properly
// handles pro server requests.
func APIHandler() http.Handler {
	log.Debugf("Returning pro API handler hitting host: %v", proAPIHost)
	return &httputil.ReverseProxy{
		Transport: &proxyTransport{},
		Director: func(r *http.Request) {
			// Strip /pro from path.
			if strings.HasPrefix(r.URL.Path, "/pro/") {
				r.URL.Path = r.URL.Path[4:]
			}
			r.URL.Scheme = "https"
			r.URL.Host = proAPIHost
			r.Host = r.URL.Host
			r.RequestURI = "" // http: Request.RequestURI can't be set in client requests.
			r.Header.Set("Lantern-Fronted-URL", fmt.Sprintf("http://%s%s", proAPIDDFHost, r.URL.Path))
			r.Header.Set("Access-Control-Allow-Headers", "X-Lantern-Device-Id, X-Lantern-Pro-Token, X-Lantern-User-Id")
		},
	}
}
