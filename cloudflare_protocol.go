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

const (
	CF_PREFIX        = "Cf-"
	X_LANTERN_PREFIX = "X-Lantern-"
	X_LANTERN_URL    = X_LANTERN_PREFIX + "URL"
)

// cloudFlareServerProtocol implements serverProtocol using CloudFlare
type cloudFlareServerProtocol struct {
}

// cloudFlareClientProtocol implements clientProtocol using CloudFlare
type cloudFlareClientProtocol struct {
	upstreamHost   string
	cloudFlareHost string
	upstreamAddr   string
}

func newCloudFlareClientProtocol(upstreamHost string, upstreamPort int, masqueradeAs string) *cloudFlareClientProtocol {
	cloudFlareHost := upstreamHost
	if masqueradeAs != "" {
		cloudFlareHost = masqueradeAs
	}
	return &cloudFlareClientProtocol{
		upstreamHost:   upstreamHost,
		cloudFlareHost: cloudFlareHost,
		upstreamAddr:   fmt.Sprintf("%s:%d", cloudFlareHost, upstreamPort),
	}
}

func (cf *cloudFlareClientProtocol) rewriteRequest(req *http.Request) {
	// Remember the host and scheme that was actually requested
	req.Header.Set(X_LANTERN_URL, req.URL.String())
	req.URL.Scheme = "http"

	// Set our upstream proxy as the host for this request
	req.Host = cf.upstreamHost
	req.URL.Host = cf.upstreamHost
}

func (cf *cloudFlareClientProtocol) rewriteResponse(resp *http.Response) {
}

func (cf *cloudFlareClientProtocol) dial(addr string) (net.Conn, error) {
	tlsConfig := &tls.Config{
		RootCAs: masqueradeCACertPool,
	}
	log.Printf("Using %s to handle request", cf.upstreamAddr)
	return tls.Dial("tcp", cf.upstreamAddr, tlsConfig)
}

func newCloudFlareServerProtocol() *cloudFlareServerProtocol {
	return &cloudFlareServerProtocol{}
}

func (cf *cloudFlareServerProtocol) rewriteRequest(req *http.Request) {
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
	req.Header.Del("X-Forwarded-Proto")

	// Strip the X-Forwarded-For header to avoid leaking the client's IP address
	req.Header.Del("X-Forwarded-For")
}

func (cf *cloudFlareServerProtocol) rewriteResponse(resp *http.Response) {
}
