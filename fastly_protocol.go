package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// fastlyServerProtocol implements serverProtocol using Fastly (not yet working)
type fastlyServerProtocol struct {
}

// fastlyServerProtocol implements clientProtocol using Fastly (not yet working)
type fastlyClientProtocol struct {
	upstreamHost string
	fastlyHost   string
	upstreamAddr string
}

func newfastlyClientProtocol(upstreamHost string, upstreamPort int, masqueradeAs string) *fastlyClientProtocol {
	fastlyHost := upstreamHost
	if masqueradeAs != "" {
		fastlyHost = masqueradeAs
	}
	return &fastlyClientProtocol{
		upstreamHost: upstreamHost,
		fastlyHost:   fastlyHost,
		upstreamAddr: fmt.Sprintf("%s:%d", fastlyHost, upstreamPort),
	}
}

func (cf *fastlyClientProtocol) rewriteRequest(req *http.Request) {
	// Remember the host and scheme that was actually requested
	req.Header.Set(X_LANTERN_URL, req.URL.String())
	req.URL.Scheme = "http"

	// Set our upstream proxy as the host for this request
	req.Host = cf.upstreamHost
	req.URL.Host = cf.upstreamHost
}

func (cf *fastlyClientProtocol) rewriteResponse(resp *http.Response) {
}

func (cf *fastlyClientProtocol) dial(addr string) (net.Conn, error) {
	log.Printf("Using %s to handle request", cf.upstreamAddr)

	// Manually dial and upgrade to TLS to avoid logic in tls.Dial() that
	// defaults the ServerName based on the host being dialed.
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	conn := tls.Client(c, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err = conn.Handshake(); err != nil {
		c.Close()
		return nil, err
	}
	return conn, nil
}

func newfastlyServerProtocol() *fastlyServerProtocol {
	return &fastlyServerProtocol{}
}

func (cf *fastlyServerProtocol) rewriteRequest(req *http.Request) {
	// Grab the original URL as passed via the X_LANTERN_URL header
	url, err := url.Parse(req.Header.Get(X_LANTERN_URL))
	if err != nil {
		log.Printf("Unable to parse URL from downstream! %s", err)
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

func (cf *fastlyServerProtocol) rewriteResponse(resp *http.Response) {
}
