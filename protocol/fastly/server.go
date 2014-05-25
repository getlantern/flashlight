package fastly

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/getlantern/flashlight/log"
)

// FastlyServerProtocol implements serverProtocol using Fastly (not yet working)
type FastlyServerProtocol struct {
}

func NewServerProtocol() *FastlyServerProtocol {
	return &FastlyServerProtocol{}
}

func (cf *FastlyServerProtocol) RewriteRequest(req *http.Request) {
	// Grab the original URL as passed via the X_LANTERN_URL header
	url, err := url.Parse(req.Header.Get(X_LANTERN_URL))
	if err != nil {
		log.Errorf("Unable to parse URL from downstream! %s", err)
		return
	}
	req.URL = url

	// Grab the host from the URL
	req.Host = req.URL.Host

	// Strip all fastly and Lantern headers
	for key, _ := range req.Header {
		shouldStrip := strings.Index(key, X_LANTERN_PREFIX) == 0 ||
			strings.Index(key, CF_PREFIX) == 0
		if shouldStrip {
			delete(req.Header, key)
		}
	}

	// Strip X-Forwarded-Proto header
	req.Header.Del("X-Forwarded-Proto")

	// Strip the X-Forwarded-For header to avoid leaking the client's IP address
	req.Header.Del("X-Forwarded-For")
}

func (cf *FastlyServerProtocol) RewriteResponse(resp *http.Response) {
}
