package client

import (
	"net/http"
	"strings"
	"time"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
)

// ServeHTTP implements the http.Handler interface.
func (client *Client) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	isConnect := req.Method == http.MethodConnect
	if isConnect {
		// Add the scheme back for CONNECT requests. It is cleared
		// intentionally by the standard library, see
		// https://golang.org/src/net/http/request.go#L938. The easylist
		// package and httputil.DumpRequest require the scheme to be present.
		req.URL.Scheme = "http"
	}

	log.Debugf("Serving HTTP for request %v", req)
	log.Debugf("Serving HTTP for request %v", req.URL.Host)
	if req.URL.Host == "ui.lantern.io" {
		log.Debug("GOT LANTERN UI!!!")
	}

	if strings.HasPrefix(req.URL.Host, "ui.lantern.io") {
		log.Debug("GOT LANTERN UI PREFIX!!!")
		req.URL.Host = "127.0.0.1"
		req.Header.Set("Host", "127.0.0.1")
	}

	if !client.easylist.Allow(req) {
		client.easyblock(resp, req)
		return
	}

	userAgent := req.Header.Get("User-Agent")
	op := ops.Begin("proxy").
		UserAgent(userAgent).
		OriginFromRequest(req)
	defer op.End()

	if isConnect {
		// CONNECT requests are often used for HTTPS requests.
		log.Tracef("Intercepting CONNECT %s", req.URL)
		err := client.interceptCONNECT(resp, req)
		if err != nil {
			log.Error(op.FailIf(err))
		}
	} else {
		log.Debugf("Checking for HTTP redirect for %v", req.URL.String())
		if httpsURL, changed := client.rewriteToHTTPS(req.URL); changed {
			client.redirect(resp, req, httpsURL, op)
			return
		}
		// Direct proxying can only be used for plain HTTP connections.
		log.Debugf("Intercepting HTTP request %s %v", req.Method, req.URL)
		// consumed and removed by http-proxy-lantern/versioncheck
		req.Header.Set(common.VersionHeader, common.Version)
		err := client.interceptHTTP(resp, req)
		if err != nil {
			log.Error(op.FailIf(err))
		}
	}
}

func (client *Client) easyblock(resp http.ResponseWriter, req *http.Request) {
	log.Debugf("Blocking %v on %v", req.URL, req.Host)
	client.statsTracker.IncAdsBlocked()
	resp.WriteHeader(http.StatusForbidden)
}

func (client *Client) redirect(resp http.ResponseWriter, req *http.Request, httpsURL string, op *ops.Op) {
	log.Debugf("httpseverywhere redirecting to %v", httpsURL)
	op.Set("forcedhttps", true)
	client.statsTracker.IncHTTPSUpgrades()
	// Tell the browser to only cache the redirect for a day. The browser
	// is generally caches permanent redirects, well, permanently but will also
	// follow caching rules.
	resp.Header().Set("Cache-Control", "max-age:86400")
	resp.Header().Set("Expires", time.Now().Add(time.Duration(24)*time.Hour).Format(http.TimeFormat))
	http.Redirect(resp, req, httpsURL, http.StatusMovedPermanently)
}
