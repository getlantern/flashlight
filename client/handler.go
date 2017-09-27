package client

import (
	"context"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/ui"
	"github.com/getlantern/proxy/filters"
	"github.com/getlantern/tarfs"
)

type contextKey string

const (
	ctxKeyOp = contextKey("op")
)

var adSwapJavaScriptInjections = map[string]string{
	"http://www.googletagservices.com/tag/js/gpt.js": "https://ads.getlantern.org/v1/js/www.googletagservices.com/tag/js/gpt.js",
	"http://cpro.baidustatic.com/cpro/ui/c.js":       "https://ads.getlantern.org/v1/js/cpro.baidustatic.com/cpro/ui/c.js",
}

var (
	fs           *tarfs.FileSystem
	translations = eventual.NewValue()
)

func init() {
	// http.FileServer relies on OS to guess mime type, which can be wrong.
	// Override system default for current process.
	_ = mime.AddExtensionType(".css", "text/css")
	_ = mime.AddExtensionType(".js", "application/javascript")
	unpackUI()
}

func (client *Client) handle(conn net.Conn) error {
	op := ops.Begin("proxy")
	ctx := context.WithValue(context.Background(), ctxKeyOp, op)
	err := client.proxy.Handle(ctx, conn)
	if err != nil {
		log.Error(op.FailIf(err))
	}
	op.End()
	return err
}

func (client *Client) filter(ctx filters.Context, r *http.Request, next filters.Next) (*http.Response, filters.Context, error) {
	if r.Method == http.MethodConnect && strings.Contains(r.URL.Host, "search.lantern.io") {
		log.Debugf("FOUND FOR CONNECT -- IN HANDLER -- search.lantern.io: %+v", r)
	}

	req, err := client.requestFilter(r)
	if err != nil {
		req = r
	}
	log.Debugf("Original scheme:  %v", req.URL.Scheme)
	// Add the scheme back for CONNECT requests. It is cleared
	// intentionally by the standard library, see
	// https://golang.org/src/net/http/request.go#L938. The easylist
	// package and httputil.DumpRequest require the scheme to be present.
	req.URL.Scheme = "http"
	req.URL.Host = req.Host

	op := ctx.Value(ctxKeyOp).(*ops.Op)

	adSwapURL := client.adSwapURL(req)
	if adSwapURL == "" && !client.easylist.Allow(req) {
		// Don't record this as proxying
		op.Cancel()
		return client.easyblock(ctx, req)
	}

	op.UserAgent(req.Header.Get("User-Agent")).OriginFromRequest(req)

	if adSwapURL != "" {
		return client.redirectAdSwap(ctx, req, adSwapURL, op)
	}

	isConnect := req.Method == http.MethodConnect
	if isConnect {
		// CONNECT requests are often used for HTTPS requests.
		log.Tracef("Intercepting CONNECT %s", req.URL)
	} else {
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
					return client.redirectHTTPS(ctx, req, httpsURL, op)
				}
			}

		}
		// Direct proxying can only be used for plain HTTP connections.
		log.Tracef("Intercepting HTTP request %s %v", req.Method, req.URL)
		// consumed and removed by http-proxy-lantern/versioncheck
		req.Header.Set(common.VersionHeader, common.Version)
	}

	return next(ctx, req)
}

func unpackUI() (*tarfs.FileSystem, error) {
	var err error
	fs, err = tarfs.New(ui.Resources, "")
	if err != nil {
		// Panicking here because this shouldn't happen at runtime unless the
		// resources were incorrectly embedded.
		panic(fmt.Errorf("Unable to open tarfs filesystem: %v", err))
	}
	translations.Set(fs.SubDir("locale"))
	return fs, err
}

func (client *Client) easyblock(ctx filters.Context, req *http.Request) (*http.Response, filters.Context, error) {
	log.Debugf("Blocking %v on %v", req.URL, req.Host)
	client.statsTracker.IncAdsBlocked()
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Close:      true,
	}
	return filters.ShortCircuit(ctx, req, resp)
}

func (client *Client) redirectHTTPS(ctx filters.Context, req *http.Request, httpsURL string, op *ops.Op) (*http.Response, filters.Context, error) {
	log.Debugf("httpseverywhere redirecting to %v", httpsURL)
	op.Set("forcedhttps", true)
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
	log.Debugf("Swapping javascript for %v to %v", urlString, jsURL)
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
