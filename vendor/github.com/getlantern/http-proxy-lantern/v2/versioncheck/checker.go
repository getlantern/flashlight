// Package versioncheck checks if the X-Lantern-Version header in the request
// is below than a semantic version, and rewrite/redirect a fraction of such
// requests to a predefined URL.
//
// For CONNECT tunnels, it simply checks the X-Lantern-Version header in the
// CONNECT request, as it's ineffeicient to inspect the tunneled data
// byte-to-byte. It redirects to the predefined URL via HTTP 302 Found. Note -
// this only works for CONNECT requests whose payload isn't encrypted (i.e.
// CONNECT requests from mobile app to port 80).
//
// For GET requests, it checks if the request is come from browser (via
// User-Agent) and expects HTML content, to be more precise. It rewrites the
// request to access the predefined URL directly.
//
// It doesn't check other HTTP methods.
//
// The purpose is to show an upgrade notice to the users with outdated Lantern
// client.
//
package versioncheck

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/getlantern/golog"
	"github.com/getlantern/proxy/v2/filters"

	"github.com/getlantern/http-proxy-lantern/v2/common"
	"github.com/getlantern/http-proxy-lantern/v2/instrument"
)

var (
	log = golog.LoggerFor("versioncheck")

	random = rand.New(rand.NewSource(time.Now().UnixNano()))
)

const (
	oneMillion = 100 * 100 * 100
)

type VersionChecker struct {
	versionRange     semver.Range
	rewriteURL       *url.URL
	rewriteURLString string
	rewriteAddr      string
	tunnelPorts      []string
	ppm              int
	instrument       instrument.Instrument
}

// New constructs a VersionChecker to check the request and rewrite/redirect if
// required.  It errors if the versionRange string is not valid, or the rewrite
// URL is malformed. tunnelPortsToCheck defaults to 80 only.
func New(versionRange string, rewriteURL string, tunnelPortsToCheck []string, percentage float64, inst instrument.Instrument) (*VersionChecker, error) {
	u, err := url.Parse(rewriteURL)
	if err != nil {
		return nil, err
	}
	rewriteAddr := u.Host

	if u.Scheme == "https" {
		rewriteAddr = rewriteAddr + ":443"
	}

	if len(tunnelPortsToCheck) == 0 {
		tunnelPortsToCheck = []string{"80"}
	}
	ver, err := semver.ParseRange(versionRange)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		inst = instrument.NoInstrument{}
	}
	return &VersionChecker{ver, u, rewriteURL, rewriteAddr, tunnelPortsToCheck, int(percentage * oneMillion), inst}, nil
}

// Dial is a function that dials a network connection.
type Dial func(ctx context.Context, network, address string) (net.Conn, error)

// Dialer wraps Dial to dial TLS when the requested host matchs the host in
// rewriteURL. If the rewriteURL is not https, it returns Dial as is.
func (c *VersionChecker) Dialer(d Dial) Dial {
	if c.rewriteURL.Scheme != "https" {
		return d
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := d(ctx, network, address)
		if err != nil {
			return conn, err
		}
		if c.rewriteAddr == address {
			conn = tls.Client(conn, &tls.Config{ServerName: c.rewriteURL.Host})
		}
		return conn, err
	}
}

// Filter returns a filters.Filter interface to be used in the filter chain.
func (c *VersionChecker) Filter() filters.Filter {
	return c
}

// Apply satisfies the filters.Filter interface.
func (c *VersionChecker) Apply(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
	defer req.Header.Del(common.VersionHeader)
	// avoid redirect loop
	if req.Host == c.rewriteURL.Host {
		return next(cs, req)
	}
	var shouldRedirect bool
	var reason string
	defer func() {
		c.instrument.VersionCheck(shouldRedirect, req.Method, reason)
	}()
	switch req.Method {
	case http.MethodConnect:
		if shouldRedirect, reason = c.shouldRedirectOnConnect(req); shouldRedirect {
			return c.redirectOnConnect(cs, req)
		}
	case http.MethodGet:
		// the first request from browser should always be GET
		if shouldRedirect, reason = c.shouldRedirect(req); shouldRedirect {
			return c.redirect(cs, req)
		}
	}
	return next(cs, req)
}

func (c *VersionChecker) redirect(cs *filters.ConnectionState, req *http.Request) (*http.Response, *filters.ConnectionState, error) {
	log.Tracef("Redirecting %s %s%s to %s",
		req.Method,
		req.Host,
		req.URL.Path,
		c.rewriteURLString,
	)
	return &http.Response{
		StatusCode: http.StatusFound,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"Location": []string{c.rewriteURLString},
		},
		Close: true,
	}, cs, nil
}

func (c *VersionChecker) shouldRedirect(req *http.Request) (bool, string) {
	if !strings.HasPrefix(req.Header.Get("Accept"), "text/html") {
		return false, "not html"
	}
	if !strings.HasPrefix(req.Header.Get("User-Agent"), "Mozilla/") {
		return false, "not from browser"
	}
	return c.matchVersion(req)
}

func (c *VersionChecker) shouldRedirectOnConnect(req *http.Request) (bool, string) {
	_, port, err := net.SplitHostPort(req.Host)
	if err != nil {
		return false, "malformed host"
	}
	for _, p := range c.tunnelPorts {
		if port == p {
			return c.matchVersion(req)
		}
	}
	return false, "ineligible port"
}

func (c *VersionChecker) redirectOnConnect(cs *filters.ConnectionState, req *http.Request) (*http.Response, *filters.ConnectionState, error) {
	conn := cs.Downstream()
	// Acknowledge the CONNECT request
	resp := &http.Response{
		StatusCode: http.StatusOK,
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
	if err := resp.Write(conn); err != nil {
		return nil, cs, err
	}

	// Consume the first request the application sent over the CONNECT tunnel
	// before sending the response.
	bufReader := bufio.NewReader(conn)
	req, err := http.ReadRequest(bufReader)
	if err != nil {
		log.Tracef("Fail to read tunneled request before redirecting: %v", err)
	} else if req.Body != nil {
		_, _ = io.Copy(ioutil.Discard, req.Body)
		req.Body.Close()
	}

	// Send the actual response to the application regardless of what the
	// request is, as the request is consumed already.
	return c.redirect(cs, req)
}

func (c *VersionChecker) matchVersion(req *http.Request) (bool, string) {
	app := req.Header.Get(common.AppHeader)
	app = strings.ToLower(app)
	if app != "" && app != "lantern" {
		return false, "version check only applies to Lantern"
	}
	version := req.Header.Get(common.VersionHeader)
	if version == "" {
		return false, "no version header"
	}
	v, e := semver.Make(version)
	if e != nil {
		return false, "malformed version"
	}
	if !c.versionRange(v) {
		return false, "ineligible version"
	}
	if random.Intn(oneMillion) >= c.ppm {
		return false, "not sampled"
	}
	return true, "eligible version"
}
