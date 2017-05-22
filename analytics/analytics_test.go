package analytics

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/getlantern/eventual"
	"github.com/getlantern/golog"
	"github.com/stretchr/testify/assert"
)

func TestAnalytics(t *testing.T) {
	logger := golog.LoggerFor("flashlight.analytics_test")

	params := eventual.NewValue()
	service := New(true, "deviceID", "2.2.0").(*analytics)
	service.Configure(&ConfigOpts{"127.0.0.1"})
	// override for test purpose
	service.transport = func(args string) {
		logger.Debugf("Got args %v", args)
		params.Set(args)
	}
	service.Start()

	args, ok := params.Get(40 * time.Second)
	assert.True(t, ok)

	argString := args.(string)
	assert.True(t, strings.Contains(argString, "pageview"))
	assert.True(t, strings.Contains(argString, "127.0.0.1"))

	// Now actually hit the GA debug server to validate the hit.
	url := "https://www.google-analytics.com/debug/collect?" + argString
	resp, err := http.Get(url)
	if !assert.NoError(t, err, "Should have no error accessing GA") {
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if !assert.NoError(t, err, "Should have no error read body") {
		return
	}
	assert.True(t, strings.Contains(string(body), "\"valid\": true"), "Should be a valid hit")
}
