package services

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/getlantern/flashlight/v7/common"
)

const (
	// retryWaitMillis is the base wait time in milliseconds between retries
	retryWaitMillis = 100
	maxRetryWait    = 10 * time.Minute
)

// sender is a helper for sending post requests. If the request fails, sender calulates an
// exponential backoff time using retryWaitMillis and return it as the sleep time.
type sender struct {
	failCount      int
	atMaxRetryWait bool
}

// post posts data to the specified URL and returns the response, the sleep time in seconds, and any
// error that occurred.
//
// Note: if the request is successful, it is the responsibility of the caller to read the response
// body to completion and close it.
func (s *sender) post(
	originURL string,
	buf io.Reader,
	rt http.RoundTripper,
	user common.UserConfig,
) (*http.Response, int64, error) {
	resp, err := s.doPost(originURL, buf, rt, user)
	if err == nil {
		s.failCount = 0
		s.atMaxRetryWait = false

		if resp.StatusCode != http.StatusOK || resp.StatusCode != http.StatusNoContent {
			return nil, 0, fmt.Errorf("bad response code: %v", resp.StatusCode)
		}

		var sleepTime int64
		if sleepVal := resp.Header.Get(common.SleepHeader); sleepVal != "" {
			if sleepTime, err = strconv.ParseInt(sleepVal, 10, 64); err != nil {
				logger.Errorf("Could not parse sleep val: %v", err)
			}
		}

		return resp, sleepTime, nil
	}

	if s.atMaxRetryWait {
		// we've already reached the max wait time, so we don't need to perform the calculation again.
		// we'll still increment the fail count to keep track of the number of failures
		s.failCount++
		return nil, int64(maxRetryWait.Seconds()), err
	}

	wait := time.Duration(math.Pow(2, float64(s.failCount)) * float64(retryWaitMillis))
	wait *= time.Millisecond
	s.failCount++

	if wait > maxRetryWait {
		s.atMaxRetryWait = true
		return nil, int64(maxRetryWait.Seconds()), err
	}

	return nil, int64(wait.Seconds()), err
}

func (s *sender) doPost(
	originURL string,
	buf io.Reader,
	rt http.RoundTripper,
	user common.UserConfig,
) (*http.Response, error) {
	req, err := http.NewRequest("POST", originURL, buf)
	if err != nil {
		return nil, fmt.Errorf("unable to create request for %s: %w", originURL, err)
	}

	common.AddCommonHeaders(user, req)
	req.Header.Set("Content-Type", "application/x-protobuf")
	// Prevents intermediate nodes (domain-fronters) from caching the content
	req.Header.Set("Cache-Control", "no-cache")

	// make sure to close the connection after reading the Body
	// this prevents the occasional EOFs errors we're seeing with
	// successive requests
	req.Close = true
	resp, err := rt.RoundTrip(req)
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("request to %s failed: %w", originURL, err)
	}

	logger.Debugf("Response headers from %v:\n%v", originURL, resp.Header)
	return resp, nil
}
