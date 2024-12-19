package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/getlantern/idletiming"
	"github.com/getlantern/proxy/v3/filters"

	"github.com/getlantern/flashlight/v7/chained"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/ops"
	"github.com/getlantern/flashlight/v7/pro"
)

func (client *Client) handle(conn net.Conn) error {
	op := ops.Begin("proxy")
	client.opsMap.put(conn, op)
	defer client.opsMap.delete(conn)
	// Use idletiming on client connections to make sure we don't get dangling server connections when clients disappear without our knowledge
	conn = idletiming.Conn(conn, chained.IdleTimeout, func() {
		log.Debugf("Client connection to %v idle for %v, closed", conn.RemoteAddr(), chained.IdleTimeout)
	})
	err := client.proxy.Handle(context.Background(), conn, conn)
	if err != nil {
		log.Error(op.FailIf(err))
	}
	op.End()
	return err
}

func (client *Client) filter(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
	if client.isHTTPProxyPort(req) {
		log.Debugf("Reject proxy request to myself: %s", req.Host)
		// Not reveal any error text to the application.
		return filters.Fail(cs, req, http.StatusBadRequest, errors.New(""))
	}

	// Add the scheme back for CONNECT requests. It is cleared
	// intentionally by the standard library, see
	// https://golang.org/src/net/http/request.go#L938. The easylist
	// package and httputil.DumpRequest require the scheme to be present.
	req.URL.Scheme = "http"
	req.URL.Host = req.Host

	if common.Platform == "android" && req.URL != nil && req.URL.Host == "localhost" &&
		strings.HasPrefix(req.URL.Path, "/pro/") {
		return client.interceptProRequest(cs, req)
	}

	op, ok := client.opsMap.get(cs.Downstream())
	if ok {
		op.UserAgent(req.Header.Get("User-Agent")).OriginFromRequest(req)
	}

	isConnect := req.Method == http.MethodConnect
	if isConnect || cs.IsMITMing() {
		// CONNECT requests are often used for HTTPS requests. If we're MITMing the
		// connection, we've stripped the CONNECT and actually performed the MITM
		// at this point, so we have to check for that and skip redirecting to
		// HTTPS in that case.
		log.Tracef("Intercepting CONNECT %s", req.URL)
	} else {
		if client.allowHTTPSEverywhere() {
			log.Tracef("Checking for HTTP redirect for %v", req.URL.String())
			if httpsURL, changed := client.rewriteToHTTPS(req.URL); changed {
				// Don't redirect CORS requests as it means the HTML pages that
				// initiate the requests were not HTTPS redirected. Redirecting
				// them adds few benefits, but may break some sites.
				if origin := req.Header.Get("Origin"); origin == "" {
					// Not rewrite recently rewritten URL to avoid redirect loop.
					if t, ok := client.rewriteLRU.Get(httpsURL); ok && time.Since(t.(time.Time)) < httpsRewriteInterval {
						log.Debugf("Not httpseverywhere redirecting to %v to avoid redirect loop", httpsURL)
					} else {
						client.rewriteLRU.Add(httpsURL, time.Now())
						return client.redirectHTTPS(cs, req, httpsURL, op)
					}
				}
			}
		}
		// Direct proxying can only be used for plain HTTP connections.
		log.Tracef("Intercepting HTTP request %s %v", req.Method, req.URL)
	}

	return next(cs, req)
}

func (client *Client) isHTTPProxyPort(r *http.Request) bool {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		// In case if it listens on standard ports, though highly unlikely.
		host = r.Host
		switch r.URL.Scheme {
		case "http", "ws":
			port = "80"
		case "https", "wss":
			port = "443"
		default:
			return false
		}
	}
	if port != client.httpProxyPort {
		return false
	}
	addrs, elookup := net.LookupHost(host)
	if elookup != nil {
		return false
	}
	for _, addr := range addrs {
		if addr == client.httpProxyIP {
			return true
		}
	}
	return false
}

// interceptProRequest specifically looks for and properly handles pro server
// requests (similar to desktop's APIHandler)
func (client *Client) interceptProRequest(cs *filters.ConnectionState, r *http.Request) (*http.Response, *filters.ConnectionState, error) {
	log.Debugf("Intercepting request to pro server: %v", r.URL.Path)
	// Strip /pro from path.
	r.URL.Path = r.URL.Path[4:]
	pro.PrepareProRequest(r, client.user)
	r.Header.Del("Origin")
	resp, err := pro.HTTPClient.Do(r)
	if err != nil {
		log.Errorf("Error intercepting request to pro server: %v", err)
		resp = &http.Response{
			StatusCode: http.StatusInternalServerError,
			Close:      true,
		}
	}
	return filters.ShortCircuit(cs, r, resp)
}

func (client *Client) redirectHTTPS(cs *filters.ConnectionState, req *http.Request, httpsURL string, op *ops.Op) (*http.Response, *filters.ConnectionState, error) {
	log.Debugf("httpseverywhere redirecting to %v", httpsURL)
	if op != nil {
		op.Set("forcedhttps", true)
	}
	client.statsTracker.IncHTTPSUpgrades()
	// Tell the browser to only cache the redirect for a day. The browser
	// generally caches permanent redirects permanently, but it will obey caching
	// directives if set.
	resp := &http.Response{
		StatusCode: http.StatusMovedPermanently,
		Header:     make(http.Header, 3),
		Close:      true,
	}
	resp.Header.Set("Location", httpsURL)
	resp.Header.Set("Cache-Control", "max-age:86400")
	resp.Header.Set("Expires", time.Now().Add(time.Duration(24)*time.Hour).Format(http.TimeFormat))
	return filters.ShortCircuit(cs, req, resp)
}
