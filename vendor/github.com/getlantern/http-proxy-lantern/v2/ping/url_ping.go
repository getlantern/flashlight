package ping

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/getlantern/proxy/v2/filters"

	"github.com/getlantern/http-proxy-lantern/v2/common"
)

type urlTiming struct {
	statusCode int
	latency    time.Duration
	size       int64
	ts         time.Time
}

var (
	httpClient = http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	// We periodically expire cached timings. This should be low enough so that we
	// get up-to-date timings and don't leave outliers in the cache for too long,
	// but large enough that we're not requesting resources at a rate that looks
	// like a DDOS attack.
	defaultTimingExpiration = 1 * time.Minute
)

func (pm *pingMiddleware) urlPing(cs *filters.ConnectionState, req *http.Request, pingURL string) (*http.Response, *filters.ConnectionState, error) {
	pm.urlTimingsMx.RLock()
	timing, found := pm.urlTimings[pingURL]
	pm.urlTimingsMx.RUnlock()
	if found {
		log.Tracef("Returning existing timing for %v", pingURL)
		// Simulate latency by sleeping
		time.Sleep(timing.latency)
	} else {
		log.Tracef("Pinging %v", pingURL)
		var err error
		timing, err = pm.timeURL(pingURL)
		if err != nil {
			return filters.Fail(cs, req, http.StatusInternalServerError, log.Errorf("Unable to obtain timing for %v: %v", pingURL, err))
		}
	}

	return filters.ShortCircuit(cs, req, &http.Response{
		StatusCode: timing.statusCode,
		Header:     http.Header{common.PingTSHeader: []string{timing.ts.String()}},
	})
}

func (pm *pingMiddleware) timeURL(pingURL string) (*urlTiming, error) {
	start := time.Now()
	resp, err := httpClient.Get(pingURL)
	if err != nil {
		return nil, err
	}
	var size int64
	if resp.Body != nil {
		defer resp.Body.Close()
		size, err = io.Copy(ioutil.Discard, resp.Body)
		if err != nil {
			return nil, fmt.Errorf("Error copying response body: %v", err)
		}
	}
	latency := time.Now().Sub(start)
	timing := &urlTiming{
		statusCode: resp.StatusCode,
		latency:    latency,
		size:       size,
		ts:         start,
	}
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		// Good status, save response
		pm.urlTimingsMx.Lock()
		pm.urlTimings[pingURL] = timing
		pm.urlTimingsMx.Unlock()
	}
	return timing, nil
}

func (pm *pingMiddleware) cleanupExpiredTimings() {
	for {
		time.Sleep(pm.timingExpiration / 2)
		now := time.Now()
		pm.urlTimingsMx.Lock()
		for url, timing := range pm.urlTimings {
			if now.Sub(timing.ts) > pm.timingExpiration {
				delete(pm.urlTimings, url)
			}
		}
		pm.urlTimingsMx.Unlock()
	}
}
