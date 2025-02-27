package config

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/getlantern/detour"

	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/ops"
)

var (
	forceCountry atomic.Value
)

// ForceCountry forces config fetches to pretend client is running in the
// given countryCode (e.g. 'cn')
func ForceCountry(countryCode string) {
	countryCode = strings.ToLower(countryCode)
	log.Debugf("Forcing config country to %v", countryCode)
	forceCountry.Store(countryCode)
}

// Fetcher is an interface for fetching config updates.
type Fetcher interface {
	fetch(string) ([]byte, time.Duration, error)
}

// fetcher periodically fetches the latest cloud configuration.
type fetcher struct {
	lastCloudConfigETag map[string]string
	user                common.UserConfig
	originURL           string
	httpClient          *http.Client
}

var noSleep = 0 * time.Second

// newHttpFetcher creates a new configuration fetcher with the specified
// interface for obtaining the user ID and token if those are populated.
func newHttpFetcher(conf common.UserConfig, httpClient *http.Client, originURL string) Fetcher {
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
		originURL:           originURL,
		httpClient:          httpClient,
	}
}

func (cf *fetcher) fetch(opName string) ([]byte, time.Duration, error) {
	op := ops.Begin(opName)
	defer op.End()
	result, sleep, err := cf.doFetch(context.Background(), op)
	return result, sleep, op.FailIf(err)
}

func (cf *fetcher) doFetch(ctx context.Context, op *ops.Op) ([]byte, time.Duration, error) {
	log.Debugf("Fetching cloud config from %v", cf.originURL)

	sleepTime := noSleep
	url := cf.originURL
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, sleepTime, fmt.Errorf("unable to construct request for cloud config at %s: %s", url, err)
	}
	if cf.lastCloudConfigETag[url] != "" {
		// Don't bother fetching if unchanged
		req.Header.Set(common.IfNoneMatchHeader, cf.lastCloudConfigETag[url])
	}

	req.Header.Set("Accept", "application/x-gzip")
	// Prevents intermediate nodes (domain-fronters) from caching the content
	req.Header.Set("Cache-Control", "no-cache")
	common.AddCommonHeaders(cf.user, req)

	_forceCountry := forceCountry.Load()
	if _forceCountry != nil {
		countryCode := _forceCountry.(string)
		log.Debugf("Forcing config country to %v", countryCode)
		req.Header.Set(common.ClientCountryHeader, countryCode)
	}

	// make sure to close the connection after reading the Body
	// this prevents the occasional EOFs errors we're seeing with
	// successive requests
	req.Close = true
	resp, err := cf.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, sleepTime, fmt.Errorf("unable to fetch cloud config at %s: %s", url, err)
	}

	sleepVal := resp.Header.Get("X-Lantern-Config-Sleep")
	if sleepVal != "" {
		seconds, err := strconv.ParseInt(sleepVal, 10, 64)
		if err != nil {
			log.Errorf("Could not parse sleep val: %v", err)
		} else {
			sleepTime = time.Duration(seconds) * time.Second
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
		return nil, sleepTime, nil
	} else if resp.StatusCode != 200 {
		op.HTTPStatusCode(resp.StatusCode)
		if dumperr != nil {
			return nil, sleepTime, fmt.Errorf("bad config response code: %v", resp.StatusCode)
		}
		return nil, sleepTime, fmt.Errorf("bad config resp:\n%v", string(dump))
	}

	op.Set("config_changed", true)
	cf.lastCloudConfigETag[url] = resp.Header.Get(common.EtagHeader)
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, sleepTime, fmt.Errorf("unable to open gzip reader: %s", err)
	}

	defer func() {
		if err := gzReader.Close(); err != nil {
			log.Errorf("Unable to close gzip reader: %v", err)
		}
	}()

	body, err := io.ReadAll(gzReader)
	if !strings.Contains(url, "global") {
		log.Debugf("Fetched proxies:\n%v", string(body))
	}
	log.Debugf("Fetched cloud config")
	return body, sleepTime, err
}
