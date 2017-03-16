package client

import (
	"net/http"

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
		h := client.https.Load().(httpseverywhere.HTTPS)
		url := req.URL.String()
		log.Debugf("Checking for HTTP redirect for %v", url)
		httpsURL, changed := h(url)
		if changed {
			log.Debugf("Redirecting to %v", httpsURL)
			http.Redirect(resp, req, httpsURL, http.StatusMovedPermanently)
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
