package analytics

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/getlantern/eventual"
	"github.com/getlantern/zaplog"
	"github.com/stretchr/testify/assert"
)

type errorTripper struct {
	request *http.Request
}

func (et *errorTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	et.request = r
	return nil, errors.New("error")
}

type successTripper struct {
	request *http.Request
}

func (st *successTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	st.request = r
	t := &http.Response{
		Body: ioutil.NopCloser(bytes.NewBufferString("Hello World")),
	}

	return t, nil
}

func TestRoundTrip(t *testing.T) {
	vals := make(url.Values, 0)
	vals.Add("v", "1")
	args := vals.Encode()
	et := &errorTripper{}
	doTrackSession(args, et)

	assert.Equal(t, "application/x-www-form-urlencoded", et.request.Header.Get("Content-Type"), "unexpected content type")

	st := &successTripper{}
	doTrackSession(args, st)

	assert.Equal(t, "application/x-www-form-urlencoded", st.request.Header.Get("Content-Type"), "unexpected content type")
}

func TestAddCampaign(t *testing.T) {
	startURL := "https://test.com"
	campaignURL, err := AddCampaign(startURL, "test-campaign", "test-content", "test-medium")
	assert.NoError(t, err, "unexpected error")
	assert.Equal(t, "https://test.com?utm_campaign=test-campaign&utm_content=test-content&utm_medium=test-medium&utm_source="+runtime.GOOS, campaignURL)

	// Now test a URL that will produce an error
	startURL = ":"
	_, err = AddCampaign(startURL, "test-campaign", "test-content", "test-medium")
	assert.Error(t, err)
}

func TestAnalytics(t *testing.T) {
	logger := zaplog.LoggerFor("flashlight.analytics_test")

	params := eventual.NewValue()
	stop := start("1", "2.2.0", func(time.Duration) string {
		return "127.0.0.1"
	}, func(args string) {
		logger.Debugf("Got args %v", args)
		params.Set(args)
	})

	args, ok := params.Get(40 * time.Second)
	assert.True(t, ok)

	argString := args.(string)
	assert.True(t, strings.Contains(argString, "pageview"))
	assert.True(t, strings.Contains(argString, "127.0.0.1"))

	// Now actually hit the GA debug server to validate the hit.
	url := "https://www.google-analytics.com/debug/collect?" + argString
	resp, err := http.Get(url)
	if !assert.NoError(t, err, "Should have no error") {
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err, "Should be nil")

	assert.True(t, strings.Contains(string(body), "\"valid\": true"), "Should be a valid hit")

	stop()
}
