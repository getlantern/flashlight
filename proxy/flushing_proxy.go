package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

const (
	X_LANTERN_STREAMING     = "X-Lantern-Streaming"
	YES                     = "yes"
	RESPONSE_FLUSH_INTERVAL = 50 * time.Millisecond
)

// flushingReverseProxy is a ReverseProxy that will periodically flush responses
// for any request that contained the header "X-Lantern-Streaming" set to "yes".
type flushingReverseProxy struct {
	reverseProxy             *httputil.ReverseProxy // the main reverse proxy
	reverseProxyWithFlushing *httputil.ReverseProxy // a copy of the main reverse proxy that flushes at regular intervals
}

func newFlushingRevereseProxy(rp *httputil.ReverseProxy) (*flushingReverseProxy, error) {
	// Make a copy of the rp, using the given flushInterval
	frp := &httputil.ReverseProxy{
		Director: rp.Director,
		// Flush the response periodically (good for streaming responses)
		FlushInterval: RESPONSE_FLUSH_INTERVAL,
	}
	if rp.Transport != nil {
		tr, ok := rp.Transport.(*http.Transport)
		if !ok {
			return nil, fmt.Errorf("newFlushingReverseProxy only works for proxies whose RoundTripper is the default http.Transport")
		}
		frp.Transport = &(*tr)
	}
	return &flushingReverseProxy{
		reverseProxy:             rp,
		reverseProxyWithFlushing: frp,
	}, nil
}

func (rp *flushingReverseProxy) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if strings.ToLower(req.Header.Get(X_LANTERN_STREAMING)) == YES {
		rp.reverseProxyWithFlushing.ServeHTTP(resp, req)
	} else {
		rp.reverseProxy.ServeHTTP(resp, req)
	}
}
