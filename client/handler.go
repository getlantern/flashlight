package client

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/status"
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
		client.ic.Intercept(resp, req, true, op.Wrapped(), 443)
	} else {
		// Direct proxying can only be used for plain HTTP connections.
		log.Debugf("Intercepting HTTP request %s %v", req.Method, req.URL)
		client.ic.Intercept(resp, req, true, op.Wrapped(), 80)
	}
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
	default:
		// We know for sure that the requested resource is not HTML page,
		// wrap the error message in http content, or http.ReverseProxy
		// will response 500 Internal Server Error instead.
		htmlerr = []byte(err.Error())
	}

	res := &http.Response{
		Body: ioutil.NopCloser(bytes.NewBuffer(htmlerr)),
	}
	res.StatusCode = http.StatusServiceUnavailable

	res.Write(w)
}
