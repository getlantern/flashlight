package config

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/v7/common"
)

func newTestUserConfig() *common.UserConfigData {
	return common.NewUserConfigData(common.DefaultAppName, "deviceID", 10, "token", nil, "en-US")
}

// TestFetcher actually fetches a config file over the network.
func TestFetcher(t *testing.T) {
	defer deleteGlobalConfig()

	// This will actually fetch the cloud config over the network.
	rt := &http.Transport{}
	configFetcher := newHttpFetcher(newTestUserConfig(), rt, common.GlobalURL)

	bytes, _, err := configFetcher.fetch("testOpName")
	assert.Nil(t, err)
	assert.True(t, len(bytes) > 200)
}

// TestStagingSetup tests to make sure our staging config flag sets the
// appropriate URLs for staging servers.
func TestStagingSetup(t *testing.T) {
	flags := make(map[string]interface{})
	flags["staging"] = false

	rt := &http.Transport{}

	var fetch *fetcher
	fetch = newHttpFetcher(newTestUserConfig(), rt, common.UserConfigURL).(*fetcher)
	assert.Equal(t, common.UserConfigURL, fetch.originURL)

	// Blank flags should mean we use the default
	flags["cloudconfig"] = ""
	fetch = newHttpFetcher(newTestUserConfig(), rt, common.UserConfigURL).(*fetcher)
	assert.Equal(t, common.UserConfigURL, fetch.originURL)

	flags["staging"] = true
	fetch = newHttpFetcher(newTestUserConfig(), rt, common.UserConfigStagingURL).(*fetcher)
	assert.Equal(t, common.UserConfigStagingURL, fetch.originURL)
}
