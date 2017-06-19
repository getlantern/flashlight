package client

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
)

var adSwapJavaScriptInjections = map[string]string{
	"http://www.googletagservices.com/tag/js/gpt.js": "https://ads.getlantern.org/v1/js/www.googletagservices.com/tag/js/gpt.js",
	"http://cpro.baidustatic.com/cpro/ui/c.js":       "https://ads.getlantern.org/v1/js/cpro.baidustatic.com/cpro/ui/c.js",
}

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

	adSwapURL := client.adSwapURL(resp, req)

	if adSwapURL == "" && !client.easylist.Allow(req) {
		client.easyblock(resp, req)
		return
	}

	userAgent := req.Header.Get("User-Agent")
	op := ops.Begin("proxy").
		UserAgent(userAgent).
		OriginFromRequest(req)
	defer op.End()

	if adSwapURL != "" {
		client.redirectAdSwap(resp, req, adSwapURL, op)
		return
	}

	if isConnect {
		// CONNECT requests are often used for HTTPS requests.
		log.Tracef("Intercepting CONNECT %s", req.URL)
		err := client.interceptCONNECT(resp, req)
		if err != nil {
			log.Error(op.FailIf(err))
		}
	} else {
		log.Tracef("Checking for HTTP redirect for %v", req.URL.String())
		if httpsURL, changed := client.rewriteToHTTPS(req.URL); changed {
			client.redirectHTTPS(resp, req, httpsURL, op)
			return
		}
		// Direct proxying can only be used for plain HTTP connections.
		log.Tracef("Intercepting HTTP request %s %v", req.Method, req.URL)
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

func (client *Client) redirectHTTPS(resp http.ResponseWriter, req *http.Request, httpsURL string, op *ops.Op) {
	log.Debugf("httpseverywhere redirecting to %v", httpsURL)
	op.Set("forcedhttps", true)
	client.statsTracker.IncHTTPSUpgrades()
	// Tell the browser to only cache the redirect for a day. The browser
	// generally caches permanent redirects permanently, but it will obey caching
	// directives if set.
	resp.Header().Set("Cache-Control", "max-age:86400")
	resp.Header().Set("Expires", time.Now().Add(time.Duration(24)*time.Hour).Format(http.TimeFormat))
	http.Redirect(resp, req, httpsURL, http.StatusMovedPermanently)
}

func (client *Client) adSwapURL(resp http.ResponseWriter, req *http.Request) string {
	urlString := req.URL.String()
	jsURL, urlFound := adSwapJavaScriptInjections[strings.ToLower(urlString)]
	if !urlFound {
		return ""
	}
	targetURL := client.adSwapTargetURL()
	if targetURL == "" {
		return ""
	}
	lang := client.lang()
	log.Debugf("Swapping javascript for %v to %v", urlString, jsURL)
	return fmt.Sprintf("%v?lang=%v&url=%v", jsURL, url.QueryEscape(lang), url.QueryEscape(targetURL))
}

func (client *Client) redirectAdSwap(resp http.ResponseWriter, req *http.Request, adSwapURL string, op *ops.Op) {
	op.Set("adswapped", true)
	http.Redirect(resp, req, adSwapURL, http.StatusTemporaryRedirect)
}
