package client

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/proxy/v2/filters"
	sc "github.com/getlantern/shortcut"
	"github.com/getlantern/yaml"
	"github.com/stretchr/testify/require"

	"github.com/getlantern/httpseverywhere"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeExoAd(t *testing.T) {
	doTestNormalizeExoAd(t, "syndication.exdynsrv.com")
	doTestNormalizeExoAd(t, "syndication.exdynsrv.com:80")
	doTestNormalizeExoAd(t, "syndication.exdynsrv.com:443")

	req, _ := http.NewRequest("GET", "http://exdynsrv.com.friendlygfw.cn", nil)
	_, ad := normalizeExoAd(req)
	assert.False(t, ad)
}

func doTestNormalizeExoAd(t *testing.T, host string) {
	req, _ := http.NewRequest("GET", "http://"+host, nil)
	qvals := req.URL.Query()
	qvals.Set("p", "https://www.somethingelse.com")
	req.URL.RawQuery = qvals.Encode()

	req2, ad := normalizeExoAd(req)
	assert.True(t, ad)
	assert.Equal(t, "https://www.getlantern.org/", req2.URL.Query().Get("p"))
}

func TestRewriteHTTPSSuccess(t *testing.T) {
	client := newClient()
	client.rewriteToHTTPS = httpseverywhere.Eager()
	req, _ := http.NewRequest("GET", "http://www.nytimes.com/", nil)
	resp, err := roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
	assert.Equal(t, "max-age:86400", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "https://www.nytimes.com/", resp.Header.Get("Location"))
	assert.True(t, len(resp.Header.Get("Expires")) > 0)
}

func TestRewriteHTTPSCORS(t *testing.T) {
	client := newClient()
	client.rewriteToHTTPS = httpseverywhere.Eager()
	req, _ := http.NewRequest("GET", "http://www.adaptec.com/", nil)
	req.Header.Set("Origin", "www.adaptec.com")
	log.Debug("Starting CORS roundtrip")
	resp, err := roundTrip(client, req)
	log.Debug("Finished CORS round trip")
	if !assert.NoError(t, err) {
		return
	}
	assert.NotEqual(t, http.StatusMovedPermanently, resp.StatusCode)
}

func TestRewriteHTTPSRedirectLoop(t *testing.T) {
	old := httpsRewriteInterval
	defer func() { httpsRewriteInterval = old }()
	httpsRewriteInterval = 100 * time.Millisecond
	client := newClient()
	client.rewriteToHTTPS = httpseverywhere.Eager()
	testURL := "http://www.hrc.org/"

	req, _ := http.NewRequest("GET", testURL, nil)
	resp, err := roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode, "should rewrite to HTTPS at first")
	assert.Equal(t, resp.Header.Get("Location"), "https://www.hrc.org/", "HTTPS Everywhere should redirect us.")

	// The following is a bit brittle because it actually sends the request to the remote
	// server, so we are beholden to what the server does. In this case, the server
	// redirects to https://www.hrc.org:443/ as of this writing, which HTTPS Everywhere does
	// not do, allowing us to differentiate between a local and a remote redirect.
	req, _ = http.NewRequest("GET", testURL, nil)
	resp, err = roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode, "second request should still hit the remote server and get redirected")
	assert.Equal(t, resp.Header.Get("Location"), "https://www.hrc.org:443/", "second request with same URL should skip HTTPS Everywhere but still be redirected from the origin server")

	time.Sleep(2 * httpsRewriteInterval)
	req, _ = http.NewRequest("GET", testURL, nil)
	resp, err = roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode, "should rewrite to HTTPS some time later")
	assert.Equal(t, resp.Header.Get("Location"), "https://www.hrc.org/", "HTTPS Everywhere should redirect us.")

}

// func TestAdSwap(t *testing.T) {
// 	client := newClient()
// 	for orig, updated := range adSwapJavaScriptInjections {
// 		req, _ := http.NewRequest("GET", orig, nil)
// 		resp, err := roundTrip(client, req)
// 		if !assert.NoError(t, err) {
// 			return
// 		}
// 		assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
// 		expectedLocation := fmt.Sprintf("%v?lang=%v&url=%v", updated, url.QueryEscape(testLang), url.QueryEscape(testAdSwapTargetURL))
// 		assert.Equal(t, expectedLocation, resp.Header.Get("Location"))
// 	}
// }

func TestRejectHTTPProxyPort(t *testing.T) {
	client := newClient()
	client.httpProxyIP, client.httpProxyPort, _ = net.SplitHostPort("127.0.0.1:4321")
	req, _ := http.NewRequest("GET", "http://127.0.0.1:4321", nil)
	assert.True(t, client.isHTTPProxyPort(req))
	req, _ = http.NewRequest("GET", "http://localhost:4321", nil)
	assert.True(t, client.isHTTPProxyPort(req))

	client.httpProxyIP, client.httpProxyPort, _ = net.SplitHostPort("127.0.0.1:80")
	req, _ = http.NewRequest("GET", "http://localhost", nil)
	assert.True(t, client.isHTTPProxyPort(req))
	req, _ = http.NewRequest("GET", "ws://localhost", nil)
	assert.True(t, client.isHTTPProxyPort(req))

	client.httpProxyIP, client.httpProxyPort, _ = net.SplitHostPort("127.0.0.1:443")
	req, _ = http.NewRequest("GET", "https://localhost", nil)
	assert.True(t, client.isHTTPProxyPort(req))
	req, _ = http.NewRequest("GET", "wss://localhost", nil)
	assert.True(t, client.isHTTPProxyPort(req))
}

func newClientForDiversion(opts *config.GoogleSearchAdsOptions) *Client {
	client, _ := NewClient(
		tempConfigDir,
		func() bool { return false },
		func() bool { return true },
		func() bool { return false },
		func() bool { return false },
		func(ctx context.Context, addr string) (sc.Method, net.IP) {
			return sc.Proxy, nil
		},
		func() bool { return true },
		func() bool { return true },
		func() bool { return false },
		func() bool { return true },
		newTestUserConfig(),
		mockStatsTracker{},
		func() bool { return true },
		func() string { return "en" },
		func() string { return "" },
		func(host string) (string, error) { return host, nil },
		func() string { return "https://tracker/ads" },
		func(category, action, label string) {},
	)
	client.googleAdsOptions = opts
	return client
}

func TestPartnerParsing(t *testing.T) {
	yml := `
featureoptions:
  googlesearchads:
    pattern: "#taw"
    block_format: >
        <div style="padding: 10px; border: 1px solid grey">
          @LINKS
          <div style="float:right;margin-bottom:10px">
            <a href="https://ads.lantern.io/about">Lantern Ads</a>
          </div>
        </div>
        ad_format: '<a href="@LINK">@TITLE</a><p>@DESCRIPTION</p>'
    partners:
      uagm:
        - name: "Ad 1"
          url: "http://usagm.gov"
          description: "Go Here Instead!"
          keywords: ["usagm", "lantern"]
          probability: 0.5
          campaign: live
        - name: "Ad 2"
          url: "http://usagm.gov/link2"
          description: "Go Here Instead2!"
          keywords: ["usagm.*", "lantern"]
          probability: 0.8
      another_partner:
        - name: "Another Partner Ad"
          url: "http://partner.com"
          description: "No, Go Here!"
          keywords: ["superdooper"]
          probability: 0.1
`
	gl := config.NewGlobal()
	require.NoError(t, yaml.Unmarshal([]byte(yml), gl))

	var opts config.GoogleSearchAdsOptions
	require.NoError(t, gl.UnmarshalFeatureOptions(config.FeatureGoogleSearchAds, &opts))

	require.Equal(t, "#taw", opts.Pattern)
	require.Len(t, opts.Partners, 2)
	require.Equal(t, "Another Partner Ad", opts.Partners["another_partner"][0].Name)
}

func TestAdDiversion(t *testing.T) {
	NotAGooglePage := "<html><body>Hello World!</body></html>"
	TestGooglePage := `<html><body><div id="taw">Some Ads For You</div></body></html>`
	ExpectedAd1 := `<html><head></head><body><div><a href="https://tracker/ads?ad_campaign=campaign&amp;ad_url=url">name</a><p>descr</p></div></body></html>`
	ExpectedAd2 := `<html><head></head><body><div><a href="https://tracker/ads?ad_campaign=campaign&amp;ad_url=url2">name2</a><p>descr</p></div></body></html>`
	ExpectedNoAd := "<html><head></head><body></body></html>"
	c := newClientForDiversion(&config.GoogleSearchAdsOptions{
		Pattern:     "#taw",
		BlockFormat: "<div>@LINKS</div>",
		AdFormat:    `<a href="@LINK">@TITLE</a><p>@DESCRIPTION</p>`,
		Partners: map[string][]config.PartnerAd{
			"Partner": {
				config.PartnerAd{
					Name:        "name",
					URL:         "url",
					Campaign:    "campaign",
					Description: "descr",
					Keywords:    []*regexp.Regexp{regexp.MustCompile("wo.*")},
					Probability: 1.0,
				},
				config.PartnerAd{
					Name:        "name2",
					URL:         "url2",
					Campaign:    "campaign",
					Description: "descr",
					Keywords:    []*regexp.Regexp{regexp.MustCompile("key")},
					Probability: 1.0,
				},
				config.PartnerAd{
					Name:        "name3",
					URL:         "url3",
					Campaign:    "campaign",
					Description: "descr",
					Keywords:    []*regexp.Regexp{regexp.MustCompile("noway")},
					Probability: 0.0,
				},
			},
		},
	})
	handlerForVar := func(v string) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			bw := brotli.NewWriter(w)
			bw.Write([]byte(v))
			bw.Close()
		}
	}

	nextForVar := func(v string) func(cs *filters.ConnectionState, req *http.Request) (*http.Response, *filters.ConnectionState, error) {
		return func(_ *filters.ConnectionState, req *http.Request) (*http.Response, *filters.ConnectionState, error) {
			w := httptest.NewRecorder()
			handlerForVar(v)(w, req)
			resp := w.Result()
			return resp, nil, nil
		}
	}
	resp, _, _ := c.divertGoogleSearchAds(nil, httptest.NewRequest("GET", "http://example.com/foo", nil), nextForVar(NotAGooglePage))
	require.NotNil(t, resp)
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, NotAGooglePage, string(body)) // when we can't detect ads - it should return the result untouched

	resp, _, _ = c.divertGoogleSearchAds(nil, httptest.NewRequest("GET", "http://example.com/foo?q=some+word", nil), nextForVar(TestGooglePage))
	require.NotNil(t, resp)
	body, _ = io.ReadAll(resp.Body)
	require.Equal(t, ExpectedAd1, string(body)) // first keyword matched by regex, show first ad

	resp, _, _ = c.divertGoogleSearchAds(nil, httptest.NewRequest("GET", "http://example.com/foo?q=key_stuff", nil), nextForVar(TestGooglePage))
	require.NotNil(t, resp)
	body, _ = io.ReadAll(resp.Body)
	require.Equal(t, ExpectedAd2, string(body)) // second keyword, second ad

	resp, _, _ = c.divertGoogleSearchAds(nil, httptest.NewRequest("GET", "http://example.com/foo?q=noway", nil), nextForVar(TestGooglePage))
	require.NotNil(t, resp)
	body, _ = io.ReadAll(resp.Body)
	require.Equal(t, ExpectedNoAd, string(body)) // third keyword, no ad since probability is 0
}
