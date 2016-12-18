package client

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/getlantern/flashlight/ops"
)

// set a hard limit when processing requests from browser. Chrome has a 30s
// timeout.
const BrowserTimeout = 29 * time.Second

func isBrowser(ua string) bool {
	return strings.HasPrefix(ua, "Mozilla") || strings.HasPrefix(ua, "Opera")
}

// ServeHTTP implements the method from interface http.Handler using the latest
// handler available from getHandler() and latest ReverseProxy available from
// getReverseProxy().
func (client *Client) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	client.serveHTTPWithContext(context.Background(), resp, req)
}

func (client *Client) serveHTTPWithContext(ctx context.Context, resp http.ResponseWriter, req *http.Request) {
	userAgent := req.Header.Get("User-Agent")
	if isBrowser(userAgent) {
		ctx, _ = context.WithTimeout(ctx, BrowserTimeout)
	}
	op := ops.Begin("proxy").
		UserAgent(userAgent).
		Origin(req)
	defer op.End()

	if req.Method == http.MethodConnect {
		// CONNECT requests are often used for HTTPS requests.
		log.Tracef("Intercepting CONNECT %s", req.URL)
		err := client.interceptCONNECT(ctx, resp, req)
		if err != nil {
			log.Error(op.FailIf(err))
		}
	} else {
		// Direct proxying can only be used for plain HTTP connections.
		log.Tracef("Intercepting HTTP request %s %v", req.Method, req.URL)
		err := client.interceptHTTP(ctx, resp, req)
		if err != nil {
			log.Error(op.FailIf(err))
		}
	}
}
