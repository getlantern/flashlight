package desktopReplica

import (
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/proxied"
)

var (
	httpClient *http.Client = genHTTPClient()
)

func genHTTPClient() *http.Client {
	return &http.Client{
		Transport: proxied.AsRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == "GET" || req.Method == "HEAD" {
				return proxied.ParallelPreferChainedWith("").RoundTrip(req)
			}
			return proxied.ChainedThenFrontedWith("").RoundTrip(req)
		}),
		// Not follow redirects
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type proxyTransport struct {
	// Satisfies http.RoundTripper
}

func (pt *proxyTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	if req.Method == "OPTIONS" {
		// No need to proxy the OPTIONS request.
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Connection":                   {"keep-alive"},
				"Access-Control-Allow-Methods": {"GET"},
				"Access-Control-Allow-Headers": {req.Header.Get("Access-Control-Request-Headers")},
				"Via":                          {"Lantern Client"},
			},
			Body: ioutil.NopCloser(strings.NewReader("preflight complete")),
		}, nil
	}

	req.Header.Del("Origin")
	resp, err = httpClient.Do(req)
	if err != nil {
		log.Errorf("Could not issue HTTP request: %v", err)
		return
	}
	return
}

func prepareRequest(r *http.Request, uc common.UserConfig, host string) {
	r.URL.Scheme = "http"
	r.URL.Host = host
	r.Host = r.URL.Host
	r.RequestURI = "" // http: Request.RequestURI can't be set in client requests.

	common.AddCommonHeaders(uc, r)
}

func proxyHandler(uc common.UserConfig, host string, modifyResponse func(*http.Response) error) http.Handler {
	return &httputil.ReverseProxy{
		Transport: &proxyTransport{},
		Director: func(r *http.Request) {
			prepareRequest(r, uc, host)
		},
		ModifyResponse: modifyResponse,
	}
}
