package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
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

func (cf *cloudFlareClientProtocol) rewrite(req *http.Request) {
	// Remember the host and scheme that was actually requested
	req.Header.Set(X_LANTERN_HOST, req.Host)
	req.Header.Set(X_LANTERN_SCHEME, req.URL.Scheme)
	req.URL.Scheme = "http"
	// Set our upstream proxy as the host for this request
	req.Host = cf.upstreamHost
	req.URL.Host = cf.upstreamHost
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

func (cf *cloudFlareServerProtocol) rewrite(req *http.Request) {
	// TODO - need to add support for tunneling HTTPS traffic using CONNECT
	req.URL.Scheme = req.Header.Get(X_LANTERN_SCHEME)
	// Grab the actual host from the original client and use that for the outbound request
	req.URL.Host = req.Header.Get(X_LANTERN_HOST)
	req.Host = req.URL.Host
}
