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
	service := New().(*analytics)
	service.Reconfigure(nil, &ConfigOpts{
		"1",
		"2.2.0",
		"127.0.0.1",
	})
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
	assert.Nil(t, err, "Should be nil")

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err, "Should be nil")

	assert.True(t, strings.Contains(string(body), "\"valid\": true"), "Should be a valid hit")
}
