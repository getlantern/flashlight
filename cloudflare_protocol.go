package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
)

const (
	X_LANTERN_HOST            = "X-Lantern-Host"
	X_LANTERN_SCHEME          = "X-Lantern-Scheme"
	X_LANTERN_TUNNELED_PREFIX = "X-Lantern-Tunneled-"
)

var (
	X_LANTERN_TUNNELED_PREFIX_LENGTH = len(X_LANTERN_TUNNELED_PREFIX)

	HOP_BY_HOP_HEADERS = map[string]bool{
		"Connection":          true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"TE":                true,
		"Trailers":          true,
		"Transfer-Encoding": true,
		"Upgrade":           true,
	}
)

type cloudFlareServerProtocol struct {
}

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
	tunnelHeaders(req.Header)

	// Remember the host and scheme that was actually requested
	req.Header.Set(X_LANTERN_HOST, req.Host)
	req.Header.Set(X_LANTERN_SCHEME, req.URL.Scheme)
	req.URL.Scheme = "http"

	// Set our upstream proxy as the host for this request
	req.Host = cf.upstreamHost
	req.URL.Host = cf.upstreamHost
}

func (cf *cloudFlareClientProtocol) rewriteResponse(resp *http.Response) {
	untunnelHeaders(resp.Header)
}

func (cf *cloudFlareClientProtocol) dial(addr string) (net.Conn, error) {
	log.Printf("Using %s to handle request", cf.upstreamAddr)
	tlsConfig := &tls.Config{
		ServerName: cf.cloudFlareHost,
	}
	return tls.Dial("tcp", cf.upstreamAddr, tlsConfig)
}

func newCloudFlareServerProtocol() *cloudFlareServerProtocol {
	return &cloudFlareServerProtocol{}
}

func (cf *cloudFlareServerProtocol) rewriteRequest(req *http.Request) {
	req.URL.Scheme = req.Header.Get(X_LANTERN_SCHEME)
	// Grab the actual host from the original client and use that for the outbound request
	req.URL.Host = req.Header.Get(X_LANTERN_HOST)

	// Remove the Lantern headers
	req.Header.Del(X_LANTERN_SCHEME)
	req.Header.Del(X_LANTERN_HOST)

	// Strip the X-Forwarded-For header to avoid leaking the client's IP address
	req.Header.Del("X-Forwarded-For")
	req.Host = req.URL.Host

	untunnelHeaders(req.Header)
}

func (cf *cloudFlareServerProtocol) rewriteResponse(resp *http.Response) {
	tunnelHeaders(resp.Header)
}

// tunnelHeaders renames headers to allow them to tunnel through CloudFlare
func tunnelHeaders(headers http.Header) {
	for key, values := range headers {
		isHopByHopHeader := !HOP_BY_HOP_HEADERS[key]
		if !isHopByHopHeader {
			prefixedKey := X_LANTERN_TUNNELED_PREFIX + key
			for _, value := range values {
				headers.Set(prefixedKey, value)
			}
		}
	}
}

// untunnelHeaders renames tunneled headers back to their normal form after
// passing through CloudFlare
func untunnelHeaders(headers http.Header) {
	for prefixedKey, values := range headers {
		if strings.Index(prefixedKey, X_LANTERN_TUNNELED_PREFIX) == 0 {
			key := prefixedKey[X_LANTERN_TUNNELED_PREFIX_LENGTH:]
			isHopByHopHeader := !HOP_BY_HOP_HEADERS[key]
			if !isHopByHopHeader {
				for _, value := range values {
					headers.Set(key, value)
				}
			}
		}
	}
}
