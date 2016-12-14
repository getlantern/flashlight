package client

import (
	"net/http"

	"github.com/getlantern/flashlight/ops"
)

// ServeHTTP implements the method from interface http.Handler using the latest
// handler available from getHandler() and latest ReverseProxy available from
// getReverseProxy().
func (client *Client) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	userAgent := req.Header.Get("User-Agent")

	easylist := client.getEasyList()
	if easylist != nil && !easylist.Allow(req) {
		log.Debugf("Blocking %v on %v", req.URL, req.Host)
		if req.Method == http.MethodConnect {
			// For CONNECT requests, we pretend that it's okay but then we don't do
			// anything afterwards. We have to do this because otherwise Chrome marks
			// us as a bad proxy.
			resp.WriteHeader(http.StatusOK)
		} else {
			resp.WriteHeader(http.StatusForbidden)
		}
		return
	}

	op := ops.Begin("proxy").
		UserAgent(userAgent).
		Origin(req)
	defer op.End()

	if req.Method == http.MethodConnect {
		// CONNECT requests are often used for HTTPS requests.
		log.Tracef("Intercepting CONNECT %s", req.URL)
		err := client.interceptCONNECT(resp, req)
		if err != nil {
			log.Error(op.FailIf(err))
		}
	} else {
		// Direct proxying can only be used for plain HTTP connections.
		log.Tracef("Intercepting HTTP request %s %v", req.Method, req.URL)
		err := client.interceptHTTP(resp, req)
		if err != nil {
			log.Error(op.FailIf(err))
		}
	}
}
