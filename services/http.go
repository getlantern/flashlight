package services

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/getlantern/flashlight/v7/common"
)

const (
	// minRetryWait is the base wait time between retries
	minRetryWait = 5 * time.Second
	maxRetryWait = 10 * time.Minute
)

// sender is a helper for sending post requests. If the request fails, sender calculates an
// exponential backoff time using minRetryWait and return it as the sleep time.
type sender struct {
	nextWait time.Duration
}

// post posts data to the specified URL and returns the response, the sleep time in seconds, and any
// error that occurred.
//
// Note: if the request is successful, it is the responsibility of the caller to read the response
// body to completion and close it.
func (s *sender) post(req *http.Request, httpClient *http.Client) (*http.Response, int64, error) {
	resp, err := s.doPost(req, httpClient)
	if err != nil {
		return resp, s.backoff(), err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		err = fmt.Errorf("bad response code: %v", resp.StatusCode)
		return resp, s.backoff(), err
	}

	s.nextWait = minRetryWait

	var sleepTime int64
	if sleepVal := resp.Header.Get(common.SleepHeader); sleepVal != "" {
		if sleepTime, err = strconv.ParseInt(sleepVal, 10, 64); err != nil {
			logger.Errorf("Could not parse sleep val: %v", err)
		}
	}

	return resp, sleepTime, nil
}

func (s *sender) doPost(req *http.Request, httpClient *http.Client) (*http.Response, error) {
	// make sure to close the connection after reading the Body
	// this prevents the occasional EOFs errors we're seeing with
	// successive requests
	req.Close = true
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to failed: %w", err)
	}

	logger.Debugf("Response status: %v headers:\n%v", resp.Status, resp.Header)
	return resp, nil
}

// backoff calculates the backoff time in seconds for the next retry.
func (s *sender) backoff() int64 {
	if s.nextWait < minRetryWait {
		s.nextWait = minRetryWait
	}

	wait := s.nextWait

	s.nextWait *= 2
	if s.nextWait > maxRetryWait {
		s.nextWait = maxRetryWait
	}

	return int64(wait.Seconds())
}
