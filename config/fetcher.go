package config

import (
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"github.com/getlantern/detour"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxied"
)

// Fetcher is an interface for fetching config updates.
type Fetcher interface {
	fetch() ([]byte, error)
}

// fetcher periodically fetches the latest cloud configuration.
type fetcher struct {
	lastCloudConfigETag map[string]string
	useLanternEtag      bool
	userConfig          common.UserConfig
	rt                  http.RoundTripper
	chainedURL          string
	frontedURL          string
}

// newFetcher creates a new configuration fetcher with the specified user ID
// and token if those are populated. Set useLanternEtag to true to always hit the
// origin server, otherwise the CDN decides if the content is changed.
func newFetcher(rt http.RoundTripper,
	useLanternEtag bool,
	userConfig common.UserConfig,
	chainedURL string,
	frontedURL string) Fetcher {
	log.Debugf("Will poll for config at %v (%v)", chainedURL, frontedURL)

	// Force detour to whitelist chained domain
	u, err := url.Parse(chainedURL)
	if err != nil {
		log.Fatalf("Unable to parse chained cloud config URL: %v", err)
	}
	detour.ForceWhitelist(u.Host)

	return &fetcher{
		lastCloudConfigETag: map[string]string{},
		useLanternEtag:      useLanternEtag,
		userConfig:          userConfig,
		rt:                  rt,
		chainedURL:          chainedURL,
		frontedURL:          frontedURL,
	}
}

func (cf *fetcher) fetch() ([]byte, error) {
	op := ops.Begin("fetch_config")
	defer op.End()
	result, err := cf.doFetch(op)
	return result, op.FailIf(err)
}

func (cf *fetcher) doFetch(op *ops.Op) ([]byte, error) {
	log.Debugf("Fetching cloud config from %v (%v)", cf.chainedURL, cf.frontedURL)

	url := cf.chainedURL
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Unable to construct request for cloud config at %s: %s", url, err)
	}
	cf.prepareRequest(req)
	resp, err := cf.rt.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("Unable to fetch cloud config at %s: %s", url, err)
	}

	defer func() {
		if closeerr := resp.Body.Close(); closeerr != nil {
			log.Errorf("Error closing response body: %v", closeerr)
		}
	}()
	dump, dumperr := httputil.DumpResponse(resp, false)
	if dumperr != nil {
		log.Errorf("Could not dump response: %v", dumperr)
	} else {
		log.Debugf("Response headers from %v (%v):\n%v", cf.chainedURL, cf.frontedURL, string(dump))
	}

	cf.storeEtag(resp)

	if resp.StatusCode == 304 {
		op.Set("config_changed", false)
		log.Debug("Config unchanged in cloud")
		return nil, nil
	} else if resp.StatusCode != 200 {
		op.HTTPStatusCode(resp.StatusCode)
		if dumperr != nil {
			return nil, fmt.Errorf("Bad config response code: %v", resp.StatusCode)
		}
		return nil, fmt.Errorf("Bad config resp:\n%v", string(dump))
	}

	op.Set("config_changed", true)
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Unable to open gzip reader: %s", err)
	}

	defer func() {
		if err := gzReader.Close(); err != nil {
			log.Errorf("Unable to close gzip reader: %v", err)
		}
	}()

	log.Debugf("Fetched cloud config")
	return ioutil.ReadAll(gzReader)
}

func (cf *fetcher) prepareRequest(req *http.Request) {
	cf.attachEtag(req)
	req.Header.Set("Accept", "application/x-gzip")
	// Prevents intermediate nodes (domain-fronters) from caching the content
	req.Header.Set("Cache-Control", "no-cache")
	// Set the fronted URL to lookup the config in parallel using chained and domain fronted servers.
	proxied.PrepareForFronting(req, cf.frontedURL)
	userID := strconv.FormatInt(cf.userConfig.GetUserID(), 10)
	req.Header.Set(common.UserIdHeader, userID)
	req.Header.Set(common.ProTokenHeader, cf.userConfig.GetToken())

	// make sure to close the connection after reading the Body
	// this prevents the occasional EOFs errors we're seeing with
	// successive requests
	req.Close = true
}

func (cf *fetcher) storeEtag(resp *http.Response) {
	var etag string
	if cf.useLanternEtag {
		etag = resp.Header.Get(common.EtagHeader)
	} else {
		etag = resp.Header.Get("Etag")
	}
	cf.lastCloudConfigETag[cf.chainedURL] = etag
}

func (cf *fetcher) attachEtag(req *http.Request) {
	etag := cf.lastCloudConfigETag[cf.chainedURL]
	if cf.useLanternEtag {
		req.Header.Set(common.IfNoneMatchHeader, etag)
	} else {
		req.Header.Set("If-None-Match", etag)
	}
}
