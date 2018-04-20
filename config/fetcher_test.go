package config

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/common"
)

func newTestUserConfig() *common.UserConfigData {
	return common.NewUserConfigData("deviceID", 10, "token", nil)
}

// TestFetcher actually fetches a config file over the network.
func TestFetcher(t *testing.T) {
	defer deleteGlobalConfig()

	// This will actually fetch the cloud config over the network.
	rt := &http.Transport{}
	configFetcher := newFetcher(newTestUserConfig(), rt, globalURLs)

	bytes, err := configFetcher.fetch()
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
	fetch = newFetcher(newTestUserConfig(), rt, proxiesURLs).(*fetcher)

	assert.Equal(t, "http://config.getiantem.org/proxies.yaml.gz", fetch.chainedURL)
	assert.Equal(t, "http://d2wi0vwulmtn99.cloudfront.net/proxies.yaml.gz", fetch.frontedURL)

	urls := proxiesURLs

	// Blank flags should mean we use the default
	flags["cloudconfig"] = ""
	flags["frontedconfig"] = ""
	fetch = newFetcher(newTestUserConfig(), rt, urls).(*fetcher)

	assert.Equal(t, "http://config.getiantem.org/proxies.yaml.gz", fetch.chainedURL)
	assert.Equal(t, "http://d2wi0vwulmtn99.cloudfront.net/proxies.yaml.gz", fetch.frontedURL)

	stagingURLs := proxiesStagingURLs
	flags["staging"] = true
	fetch = newFetcher(newTestUserConfig(), rt, stagingURLs).(*fetcher)
	assert.Equal(t, "http://config-staging.getiantem.org/proxies.yaml.gz", fetch.chainedURL)
	assert.Equal(t, "http://d33pfmbpauhmvd.cloudfront.net/proxies.yaml.gz", fetch.frontedURL)
}
