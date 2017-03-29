package client

import (
	"net/http"
	"time"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/httpseverywhere"
)

// ServeHTTP implements the http.Handler interface.
func (client *Client) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	isConnect := req.Method == http.MethodConnect
	if isConnect {
		// Add the scheme back for CONNECT requests. It is cleared
		// intentionally by the standard library, see
		// https://golang.org/src/net/http/request.go#L938. The easylist
		// package and httputil.DumpRequest require the scheme to be present.
		req.URL.Scheme = "http"
	}

	if !client.easylist.Allow(req) {
		log.Debugf("Blocking %v on %v", req.URL, req.Host)
		if isConnect {
			// For CONNECT requests, we pretend that it's okay but then we don't do
			// anything afterwards. We have to do this because otherwise Chrome marks
			// us as a bad proxy.
			resp.WriteHeader(http.StatusOK)
		} else {
			resp.WriteHeader(http.StatusForbidden)
		}
		return
	}

	userAgent := req.Header.Get("User-Agent")
	op := ops.Begin("proxy").
		UserAgent(userAgent).
		OriginFromRequest(req)
	defer op.End()

	if isConnect {
		// CONNECT requests are often used for HTTPS requests.
		log.Tracef("Intercepting CONNECT %s", req.URL)
		err := client.interceptCONNECT(resp, req)
		if err != nil {
			log.Error(op.FailIf(err))
		}
	} else {
		//url := req.URL.String()
		log.Tracef("Checking for HTTP redirect for %v", req.URL.String())
		if httpsURL, changed := httpseverywhere.Rewrite(req.URL); changed {
			client.redirect(resp, req, httpsURL, op)
			return
		}
		// Direct proxying can only be used for plain HTTP connections.
		log.Tracef("Intercepting HTTP request %s %v", req.Method, req.URL)
		err := client.interceptHTTP(resp, req)
		if err != nil {
			log.Error(op.FailIf(err))
		}
	}
}

func (client *Client) redirect(resp http.ResponseWriter, req *http.Request, httpsURL string, op *ops.Op) {
	log.Debugf("Redirecting to %v", httpsURL)
	op.Set("forcedhttps", true)
	// Tell the browser to only cache the redirect for a day. The browser
	// is generally caches permanent redirects, well, permanently but will also
	// follow caching rules.
	resp.Header().Set("Cache-Control", "max-age:86400")
	resp.Header().Set("Expires", time.Now().Add(time.Duration(24)*time.Hour).Format(http.TimeFormat))
	http.Redirect(resp, req, httpsURL, http.StatusMovedPermanently)
}
