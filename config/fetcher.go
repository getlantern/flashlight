package config

import (
	"compress/gzip"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"github.com/getlantern/detour"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
)

// Fetcher is an interface for fetching config updates.
type Fetcher interface {
	fetch(func() time.Duration) ([]byte, func() time.Duration, error)
}

// fetcher periodically fetches the latest cloud configuration.
type fetcher struct {
	lastCloudConfigETag map[string]string
	user                common.UserConfig
	rt                  http.RoundTripper
	originURL           string
}

// newFetcher creates a new configuration fetcher with the specified
// interface for obtaining the user ID and token if those are populated.
func newFetcher(conf common.UserConfig, rt http.RoundTripper, originURL string) Fetcher {
	log.Debugf("Will poll for config at %v", originURL)

	// Force detour to whitelist chained domain
	u, err := url.Parse(originURL)
	if err != nil {
		log.Fatalf("Unable to parse chained cloud config URL: %v", err)
	}
	detour.ForceWhitelist(u.Host)

	return &fetcher{
		lastCloudConfigETag: map[string]string{},
		user:                conf,
		rt:                  rt,
		originURL:           originURL,
	}
}

func (cf *fetcher) fetch(sleep func() time.Duration) ([]byte, func() time.Duration, error) {
	op, ctx := ops.BeginWithNewBeam("fetch_config", context.Background())
	defer op.End()
	result, sleepFunc, err := cf.doFetch(ctx, op, sleep)
	return result, sleepFunc, op.FailIf(err)
}

func (cf *fetcher) doFetch(ctx context.Context, op *ops.Op, sleep func() time.Duration) ([]byte, func() time.Duration, error) {
	log.Debugf("Fetching cloud config from %v", cf.originURL)

	url := cf.originURL
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to construct request for cloud config at %s: %s", url, err)
	}
	if cf.lastCloudConfigETag[url] != "" {
		// Don't bother fetching if unchanged
		req.Header.Set(common.IfNoneMatchHeader, cf.lastCloudConfigETag[url])
	}

	req.Header.Set("Accept", "application/x-gzip")
	// Prevents intermediate nodes (domain-fronters) from caching the content
	req.Header.Set("Cache-Control", "no-cache")
	common.AddCommonHeaders(cf.user, req)

	// make sure to close the connection after reading the Body
	// this prevents the occasional EOFs errors we're seeing with
	// successive requests
	req.Close = true

	resp, err := cf.rt.RoundTrip(req.WithContext(ctx))
	if err != nil {
		return nil, sleep, fmt.Errorf("Unable to fetch cloud config at %s: %s", url, err)
	}

	sleepFunc := sleep
	sleepVal := resp.Header.Get("X-Lantern-Config-Sleep")
	if sleepVal != "" {
		seconds, err := strconv.ParseInt(sleepVal, 10, 64)
		if err != nil {
			log.Errorf("Could not parse sleep val: %v", err)
		} else {
			sleepFunc = func() time.Duration {
				return time.Duration(seconds) * time.Second
			}
		}
	}

	dump, dumperr := httputil.DumpResponse(resp, false)
	if dumperr != nil {
		log.Errorf("Could not dump response: %v", dumperr)
	} else {
		log.Debugf("Response headers from %v:\n%v", cf.originURL, string(dump))
	}
	defer func() {
		if closeerr := resp.Body.Close(); closeerr != nil {
			log.Errorf("Error closing response body: %v", closeerr)
		}
	}()

	if resp.StatusCode == 304 {
		op.Set("config_changed", false)
		log.Debug("Config unchanged in cloud")
		return nil, sleepFunc, nil
	} else if resp.StatusCode != 200 {
		op.HTTPStatusCode(resp.StatusCode)
		if dumperr != nil {
			return nil, sleepFunc, fmt.Errorf("Bad config response code: %v", resp.StatusCode)
		}
		return nil, sleepFunc, fmt.Errorf("Bad config resp:\n%v", string(dump))
	}

	op.Set("config_changed", true)
	cf.lastCloudConfigETag[url] = resp.Header.Get(common.EtagHeader)
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, sleepFunc, fmt.Errorf("Unable to open gzip reader: %s", err)
	}

	defer func() {
		if err := gzReader.Close(); err != nil {
			log.Errorf("Unable to close gzip reader: %v", err)
		}
	}()

	log.Debugf("Fetched cloud config")
	body, err := ioutil.ReadAll(gzReader)
	return body, sleepFunc, err
}
