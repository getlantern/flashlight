package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getlantern/flashlight/config"

	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/brotli"

	"github.com/getlantern/idletiming"
	"github.com/getlantern/proxy/filters"

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
	op, ctx := ops.BeginWithNewBeam("proxy", context.Background())
	// Use idletiming on client connections to make sure we don't get dangling server connections when clients disappear without our knowledge
	conn = idletiming.Conn(conn, chained.IdleTimeout, func() {
		log.Debugf("Client connection idle for %v, closed", chained.IdleTimeout)
	})
	err := client.proxy.Handle(ctx, conn, conn)
	if err != nil {
		log.Error(op.FailIf(err))
	}
	op.End()
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
		log.Debugf("Reject proxy request to myself: %s", req.Host)
		// Not reveal any error text to the application.
		return filters.Fail(ctx, req, http.StatusBadRequest, errors.New(""))
	}

	trackYoutubeWatches(req)
	client.trackSearches(req)
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

	op := ops.FromContext(ctx)
	op.UserAgent(req.Header.Get("User-Agent")).OriginFromRequest(req)

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
						return client.redirectHTTPS(ctx, req, httpsURL, op)
					}
				}
			}
		}
		// Direct proxying can only be used for plain HTTP connections.
		log.Tracef("Intercepting HTTP request %s %v", req.Method, req.URL)
	}

	if client.googleAdsFilter() && strings.Contains(strings.ToLower(req.Host), ".google.") && req.Method == "GET" && req.URL.Path == "/search" {
		return client.divertGoogleSearchAds(ctx, req, next)
	}

	return next(ctx, req)
}

type PartnerAd struct {
	Title       string `json:"title"`
	Url         string `json:"url"`
	Description string `json:"description"`
	BaseUrl     string
}

func (ad PartnerAd) String(opts *config.GoogleSearchAdsOptions) string {
	link := strings.ReplaceAll(opts.AdFormat, "@ADLINK", ad.BaseUrl)
	link = strings.ReplaceAll(link, "@LINK", ad.Url)
	link = strings.ReplaceAll(link, "@TITLE", ad.Title)
	link = strings.ReplaceAll(link, "@DESCRIPTION", ad.Description)
	return link
}

type PartnerAds []PartnerAd

func (ads PartnerAds) String(client *Client, opts *config.GoogleSearchAdsOptions) string {
	if len(ads) == 0 {
		return ""
	}
	builder := strings.Builder{}

	// Randomize the ads so we don't always show the same ones.
	rand.Shuffle(len(ads), func(i, j int) {
		ads[i], ads[j] = ads[j], ads[i]
	})
	for i, ad := range ads {
		// Do not include more than a few ads.
		if i >= 2 {
			break
		}
		client.eventWithLabel("google_search_ads", "ad_injected", ad.Url)
		builder.WriteString(ad.String(opts))
	}
	return strings.Replace(opts.BlockFormat, "@LINKS", builder.String(), 1)
}

func (client *Client) generateAds(opts *config.GoogleSearchAdsOptions, keywords []string) string {
	ads := PartnerAds{}
	for _, partnerAds := range opts.Partners {
		for _, ad := range partnerAds {
			// check if any keywords match
			found := false
		out:
			for _, query := range keywords {
				client.eventWithLabel("google_search", "keyword", query)
				for _, kw := range ad.Keywords {
					if kw.MatchString(query) {
						client.eventWithLabel("google_search_ads", "keyword_match", kw.String())

						found = true
						break out
					}
				}
			}
			// we randomly skip the injection based on probability specified in config
			if found && rand.Float32() < ad.Probability {
				ads = append(ads, PartnerAd{
					Title:       ad.Name,
					Url:         client.getTrackAdUrl(ad.URL, ad.Campaign),
					Description: ad.Description,
					BaseUrl:     client.getBaseUrl(ad.URL),
				})
			}

		}
	}
	return ads.String(client, opts)
}

// getBaseUrl returns the URL for the base domain of an ad without the full path, query string,
// etc.
func (client *Client) getBaseUrl(originalUrl string) string {
	url, err := url.Parse(originalUrl)
	if err != nil {
		return originalUrl
	}
	return url.Scheme + "://" + url.Host
}

// getTrackAdUrl takes original URL and passes it to the analytics tracker
// once the click event is logged, we redirect to the actual ad
func (client *Client) getTrackAdUrl(originalUrl string, campaign string) string {
	newUrl, err := url.Parse(client.adTrackUrl())
	if err != nil {
		return originalUrl
	}
	q := newUrl.Query()
	q.Set("ad_url", originalUrl)
	if campaign != "" {
		q.Set("ad_campaign", campaign)
	}
	newUrl.RawQuery = q.Encode()
	return newUrl.String()
}

func (client *Client) divertGoogleSearchAds(initialContext filters.Context, req *http.Request, next filters.Next) (resp *http.Response, ctx filters.Context, err error) {
	log.Debug("Processing google search ads")
	client.googleAdsOptionsLock.RLock()
	opts := client.googleAdsOptions
	client.googleAdsOptionsLock.RUnlock()
	resp, ctx, err = next(initialContext, req)

	// if both requests succeed, extract partner ads and inject them
	if err == nil && opts != nil {
		body := bytes.NewBuffer(nil)
		if _, err = io.Copy(body, brotli.NewReader(resp.Body)); err != nil {
			return
		}
		_ = resp.Body.Close()
		resp.Body = ioutil.NopCloser(bytes.NewBufferString(body.String())) // restore the body, since we consumed it. In case we don't modify anything
		var doc *goquery.Document
		doc, err = goquery.NewDocumentFromReader(body)
		if err != nil {
			return
		}

		// find existing ads based on the pattern in config
		adNode := doc.Find(opts.Pattern)
		if len(adNode.Nodes) == 0 {
			return
		}

		// extract search query
		var query string
		query, err = url.QueryUnescape(req.URL.Query().Get("q"))
		if err != nil {
			return
		}
		// inject new ads
		adNode.ReplaceWithHtml(client.generateAds(opts, strings.Split(query, " ")))

		// serialize DOM to string
		var htmlResult string
		htmlResult, err = doc.Html()
		if err != nil {
			return
		}

		resp.Header.Del("Content-Encoding")
		resp.Body = io.NopCloser(bytes.NewBufferString(htmlResult))
	}
	return
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
	log.Debugf("Intercepting request to pro server: %v", r.URL.Path)
	r.URL.Path = r.URL.Path[4:]
	pro.PrepareProRequest(r, client.user)
	r.Header.Del("Origin")
	resp, err := pro.GetHTTPClient().Do(r.WithContext(ctx))
	if err != nil {
		log.Errorf("Error intercepting request to pro server: %v", err)
		resp = &http.Response{
			StatusCode: http.StatusInternalServerError,
			Close:      true,
		}
	}
	return filters.ShortCircuit(ctx, r, resp)
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

type SearchEngine struct {
	host string
	path string
}

var SearchEnginesToTrack = map[string]SearchEngine{
	"google": {
		host: "google.com",
		path: "/search",
	},
	"bing": {
		host: "bing.com",
		path: "/search",
	},
	"baidu": {
		host: "baidu.com",
		path: "/s",
	},
}

func (s *SearchEngine) Matches(req *http.Request) bool {
	if !strings.Contains(strings.ToLower(req.Host), s.host) && !strings.Contains(strings.ToLower(req.URL.Host), s.host) {
		return false
	}
	return strings.Contains(strings.ToLower(req.URL.Path), s.path)
}

func (client *Client) trackSearches(req *http.Request) {
	for engine, params := range SearchEnginesToTrack {
		if params.Matches(req) {
			client.eventWithLabel("search", "search_performed", engine)
			break
		}
	}
}

func trackYoutubeWatches(req *http.Request) {
	video := youtubeVideoFor(req)
	if video != "" {
		op := ops.Begin("youtube_view").Set("video", video)
		defer op.End()
		log.Debugf("Requested YouTube video")
	}
}

func youtubeVideoFor(req *http.Request) string {
	if !strings.Contains(strings.ToLower(req.Host), "youtube") && !strings.Contains(strings.ToLower(req.URL.Host), "youtube") {
		// not a youtube domain
		return ""
	}
	if req.URL.Path != "/watch" {
		// not a watch url
		return ""
	}
	candidate := req.URL.Query().Get("v")
	if len(candidate) < 11 {
		// invalid/corrupt video id
		return ""
	}
	return candidate[0:11]
}
