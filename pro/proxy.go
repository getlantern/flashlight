package pro

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/util"
	"github.com/getlantern/golog"
)

var (
	log        = golog.LoggerFor("flashlight.pro")
	httpClient = &http.Client{Transport: proxied.ChainedThenFronted()}
	// Respond sooner if chained proxy is blocked, but only for idempotent requests (GETs)
	httpClientForGET = &http.Client{Transport: proxied.ParallelPreferChained()}
	proAPIHost       = "api.getiantem.org"
	proAPIDDFHost    = "d2n32kma9hyo9f.cloudfront.net"
)

type proxyTransport struct {
	// Satisfies http.RoundTripper
}

// InitProxy starts the proxy listening on the specified host and port.
func InitProxy(addr string, staging bool) error {
	if staging {
		proAPIHost = "api-staging.getiantem.org"
		proAPIDDFHost = "d16igwq64x5e11.cloudfront.net"
	}
	srv := &http.Server{
		Addr:         addr,
		Handler:      util.NoCacheHandler(proxyHandler),
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	return srv.ListenAndServe()
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

var proxyHandler = &httputil.ReverseProxy{
	Transport: &proxyTransport{},
	Director: func(r *http.Request) {
		r.URL.Scheme = "https"
		r.URL.Host = proAPIHost
		r.Host = r.URL.Host
		r.RequestURI = "" // http: Request.RequestURI can't be set in client requests.
		r.Header.Set("Lantern-Fronted-URL", fmt.Sprintf("http://%s%s", proAPIDDFHost, r.URL.Path))
		r.Header.Set("Access-Control-Allow-Headers", "X-Lantern-Device-Id, X-Lantern-Pro-Token, X-Lantern-User-Id")
	},
}
