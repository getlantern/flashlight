package services

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/getlantern/golog"

	"github.com/getlantern/flashlight/v7/common"
)

// it is the caller's responsibility to read the response body to completion and close it
func post(
	originURL string,
	buf io.Reader,
	rt http.RoundTripper,
	user common.UserConfig,
	logger golog.Logger,
) (io.ReadCloser, int64, error) {
	req, err := http.NewRequest("POST", originURL, buf)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to create request for %s: %w", originURL, err)
	}

	common.AddCommonHeaders(user, req)
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Accept", "application/x-gzip")
	// Prevents intermediate nodes (domain-fronters) from caching the content
	req.Header.Set("Cache-Control", "no-cache")

	// make sure to close the connection after reading the Body
	// this prevents the occasional EOFs errors we're seeing with
	// successive requests
	req.Close = true
	resp, err := rt.RoundTrip(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request to %s failed: %w", originURL, err)
	}

	if resp.StatusCode != 200 {
		return nil, 0, fmt.Errorf("bad response code: %v", resp.StatusCode)
	}

	// TODO: do we need to log all the response headers?
	logger.Debugf("Response headers from %v:\n%v", originURL, resp.Header)

	var sleepTime int64
	sleepVal := resp.Header.Get(common.SleepHeader)
	if sleepVal != "" {
		if sleepTime, err = strconv.ParseInt(sleepVal, 10, 64); err != nil {
			logger.Errorf("Could not parse sleep val: %v", err)
		}
	}

	return resp.Body, sleepTime, nil
}
