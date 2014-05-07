package cloudflare

import (
	"log"
	"net/http"
	"net/url"
	"strings"
)

// CloudFlareServerProtocol implements serverProtocol using CloudFlare
type CloudFlareServerProtocol struct {
}

func NewServerProtocol() *CloudFlareServerProtocol {
	return &CloudFlareServerProtocol{}
}

func (cf *CloudFlareServerProtocol) RewriteRequest(req *http.Request) {
	// Grab the original URL as passed via the X_LANTERN_URL header
	url, err := url.Parse(req.Header.Get(X_LANTERN_URL))
	if err != nil {
		log.Printf("Unable to parse URL from downstream! %s", err)
		return
	}
	req.URL = url

	// Grab the host from the URL
	req.Host = req.URL.Host

	// Strip all CloudFlare and Lantern headers
	for key, _ := range req.Header {
		shouldStrip := strings.Index(key, X_LANTERN_PREFIX) == 0 ||
			strings.Index(key, CF_PREFIX) == 0
		if shouldStrip {
			delete(req.Header, key)
		}
	}

	// Strip X-Forwarded-Proto header
	req.Header.Del(X_FORWARDED_PROTO)

	// Strip the X-Forwarded-For header to avoid leaking the client's IP address
	req.Header.Del(X_FORWARDED_FOR)
}

func (cf *CloudFlareServerProtocol) RewriteResponse(resp *http.Response) {
}
