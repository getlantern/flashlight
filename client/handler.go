package client

import (
	"context"
	"net/http"
	"time"

	"github.com/getlantern/flashlight/ops"
)

// Set a hard limit when processing proxy requests. Should be short enough to
// avoid applications bypassing Lantern.
// Chrome has a 30s timeout before marking proxy as bad.
// Firefox: network.proxy.failover_timeout defaults to 1800.
const requestTimeout = 20 * time.Second

// ServeHTTP implements the method from interface http.Handler using the latest
// handler available from getHandler() and latest ReverseProxy available from
// getReverseProxy().
func (client *Client) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	req = req.WithContext(ctx)
	client.serveHTTPWithContext(ctx, resp, req)
	// To release resources, see https://golang.org/pkg/context/#WithTimeout
	cancel()
}

func (client *Client) serveHTTPWithContext(ctx context.Context, resp http.ResponseWriter, req *http.Request) {
	userAgent := req.Header.Get("User-Agent")
	op := ops.Begin("proxy").
		UserAgent(userAgent).
		OriginFromRequest(req)
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
