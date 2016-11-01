package client

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/status"
	"github.com/getlantern/hidden"
)

var (
	keepaliveIdleTimeout = fmt.Sprintf("timeout: %d", int(idleTimeout.Seconds())-2)
)

// ServeHTTP implements the method from interface http.Handler using the latest
// handler available from getHandler() and latest ReverseProxy available from
// getReverseProxy().
func (client *Client) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	userAgent := req.Header.Get("User-Agent")

	op := ops.Begin("proxy").
		UserAgent(userAgent).
		Origin(req.Host)
	defer op.End()

	if req.Method == http.MethodConnect {
		// CONNECT requests are often used for HTTPS requests.
		log.Tracef("Intercepting CONNECT %s", req.URL)
		client.interceptCONNECT(op.Wrapped(), resp, req)
	} else {
		// Direct proxying can only be used for plain HTTP connections.
		log.Tracef("Intercepting HTTP request %s %v", req.Method, req.URL)
		client.interceptHTTP(op.Wrapped(), resp, req)
	}
}

func addIdleKeepAliveHeader(resp *http.Response) *http.Response {
	// Tell the client when we're going to time out due to idle connections
	resp.Header.Set("Keep-Alive", keepaliveIdleTimeout)
	return resp
}

func errorResponse(w io.Writer, req *http.Request, err error) {
	var htmlerr []byte

	// If the request has an 'Accept' header preferring HTML, or
	// doesn't have that header at all, render the error page.
	switch req.Header.Get("Accept") {
	case "text/html":
		fallthrough
	case "application/xhtml+xml":
		fallthrough
	case "":
		// It is likely we will have lots of different errors to handle but for now
		// we will only return a ErrorAccessingPage error.  This prevents the user
		// from getting just a blank screen.
		htmlerr, err = status.ErrorAccessingPage(req.Host, err)
		if err != nil {
			log.Debugf("Got error while generating status page: %q", err)
		}
	}

	if htmlerr == nil {
		// Default value for htmlerr
		htmlerr = []byte(hidden.Clean(err.Error()))
	}

	res := &http.Response{
		Body: ioutil.NopCloser(bytes.NewBuffer(htmlerr)),
	}
	res.StatusCode = http.StatusServiceUnavailable

	res.Write(w)
}
