package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getlantern/idletiming"
	"github.com/getlantern/proxy/filters"

	"github.com/opentracing/opentracing-go"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/pro"
)

var adSwapJavaScriptInjections = map[string]string{
	"http://www.googletagservices.com/tag/js/gpt.js": "https://ads.getlantern.org/v1/js/www.googletagservices.com/tag/js/gpt.js",
	"http://cpro.baidustatic.com/cpro/ui/c.js":       "https://ads.getlantern.org/v1/js/cpro.baidustatic.com/cpro/ui/c.js",
}

func (client *Client) handle(conn net.Conn) error {
	span := opentracing.StartSpan("handleConn")
	ctx := opentracing.ContextWithSpan(context.Background(), span)

	defer span.Finish()

	// Set user agent connection to idle a little before the upstream connection
	// so that we don't read data from the client after the upstream connection
	// has already timed out.
	conn = idletiming.Conn(conn, chained.IdleTimeout-1*time.Second, nil)
	err := client.proxy.Handle(ctx, conn, conn)
	if err != nil {
		//client.log.Error(op.FailIf(err))
		client.log.Errorf("Could not handle request: %v", err)
	}
	//op.End()
	return err
}

func normalizeExoAd(req *http.Request) (*http.Request, bool) {
	host, _, err := net.SplitHostPort(req.Host)
	if err != nil {
		host = req.Host
	}
	if strings.HasSuffix(host, ".exdynsrv.com") {
		qvals := req.URL.Query()
		qvals.Set("p", "https://www.getlantern.org/")
		req.URL.RawQuery = qvals.Encode()
		return req, true
	}
	return req, false
}

func (client *Client) filter(ctx filters.Context, req *http.Request, next filters.Next) (*http.Response, filters.Context, error) {
	if client.isHTTPProxyPort(req) {
		client.log.Debugf("Reject proxy request to myself: %s", req.Host)
		// Not reveal any error text to the application.
		return filters.Fail(ctx, req, http.StatusBadRequest, errors.New(""))
	}

	// Add the scheme back for CONNECT requests. It is cleared
	// intentionally by the standard library, see
	// https://golang.org/src/net/http/request.go#L938. The easylist
	// package and httputil.DumpRequest require the scheme to be present.
	req.URL.Scheme = "http"
	req.URL.Host = req.Host

	if common.Platform == "android" && req.URL != nil && req.URL.Host == "localhost" &&
		strings.HasPrefix(req.URL.Path, "/pro/") {
		return client.interceptProRequest(ctx, req)
	}

	/*
		op := ops.FromContext(ctx)
		op.UserAgent(req.Header.Get("User-Agent")).OriginFromRequest(req)
	*/

	// Disable Ad swapping for now given that Ad blocking is completely
	// removed.  A limited form of Ad blocking should be re-introduced before
	// enabling it again.
	//
	// adSwapURL := client.adSwapURL(req)
	// if !exoclick && adSwapURL != "" {
	// // Don't record this as proxying
	// 	op.Cancel()
	// 	return client.redirectAdSwap(ctx, req, adSwapURL, op)
	// }

	isConnect := req.Method == http.MethodConnect
	if isConnect || ctx.IsMITMing() {
		// CONNECT requests are often used for HTTPS requests. If we're MITMing the
		// connection, we've stripped the CONNECT and actually performed the MITM
		// at this point, so we have to check for that and skip redirecting to
		// HTTPS in that case.
		client.log.Tracef("Intercepting CONNECT %s", req.URL)
	} else {
		client.log.Tracef("Checking for HTTP redirect for %v", req.URL.String())
		if httpsURL, changed := client.rewriteToHTTPS(req.URL); changed {
			// Don't redirect CORS requests as it means the HTML pages that
			// initiate the requests were not HTTPS redirected. Redirecting
			// them adds few benefits, but may break some sites.
			if origin := req.Header.Get("Origin"); origin == "" {
				// Not rewrite recently rewritten URL to avoid redirect loop.
				if t, ok := client.rewriteLRU.Get(httpsURL); ok && time.Since(t.(time.Time)) < httpsRewriteInterval {
					client.log.Debugf("Not httpseverywhere redirecting to %v to avoid redirect loop", httpsURL)
				} else {
					client.rewriteLRU.Add(httpsURL, time.Now())
					return client.redirectHTTPS(ctx, req, httpsURL)
				}
			}

		}
		// Direct proxying can only be used for plain HTTP connections.
		client.log.Tracef("Intercepting HTTP request %s %v", req.Method, req.URL)
		// consumed and removed by http-proxy-lantern/versioncheck
		req.Header.Set(common.VersionHeader, common.Version)
	}

	return next(ctx, req)
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
func (client *Client) interceptProRequest(ctx filters.Context, r *http.Request) (*http.Response, filters.Context, error) {
	client.log.Debugf("Intercepting request to pro server: %v", r.URL.Path)
	r.URL.Path = r.URL.Path[4:]
	pro.PrepareProRequest(r, client.user)
	r.Header.Del("Origin")
	resp, err := pro.GetHTTPClient().Do(r.WithContext(ctx))
	if err != nil {
		client.log.Errorf("Error intercepting request to pro server: %v", err)
		resp = &http.Response{
			StatusCode: http.StatusInternalServerError,
			Close:      true,
		}
	}
	return filters.ShortCircuit(ctx, r, resp)
}

func (client *Client) easyblock(ctx filters.Context, req *http.Request) (*http.Response, filters.Context, error) {
	client.log.Debugf("Blocking %v on %v", req.URL, req.Host)
	client.statsTracker.IncAdsBlocked()
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Close:      true,
	}
	return filters.ShortCircuit(ctx, req, resp)
}

func (client *Client) redirectHTTPS(ctx filters.Context, req *http.Request, httpsURL string) (*http.Response, filters.Context, error) {
	client.log.Debugf("httpseverywhere redirecting to %v", httpsURL)
	//op.Set("forcedhttps", true)
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
	return filters.ShortCircuit(ctx, req, resp)
}

func (client *Client) adSwapURL(req *http.Request) string {
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
	client.log.Debugf("Swapping javascript for %v to %v", urlString, jsURL)
	extra := ""
	if common.ForceAds() {
		extra = "&force=true"
	}
	return fmt.Sprintf("%v?lang=%v&url=%v%v", jsURL, url.QueryEscape(lang), url.QueryEscape(targetURL), extra)
}

func (client *Client) redirectAdSwap(ctx filters.Context, req *http.Request, adSwapURL string, op *ops.Op) (*http.Response, filters.Context, error) {
	op.Set("adswapped", true)
	resp := &http.Response{
		StatusCode: http.StatusTemporaryRedirect,
		Header:     make(http.Header, 1),
		Close:      true,
	}
	resp.Header.Set("Location", adSwapURL)
	return filters.ShortCircuit(ctx, req, resp)
}
