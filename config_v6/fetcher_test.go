package config

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/common"
)

func newTestUserConfig() *common.UserConfigData {
	return common.NewUserConfigData(common.DefaultAppName, "deviceID", 10, "token", nil, "en-US")
}

// TestFetcher actually fetches a config file over the network.
func TestFetcher(t *testing.T) {
	defer deleteGlobalConfig()

	// This will actually fetch the cloud config over the network.
	rt := &http.Transport{}
	configFetcher := newFetcher(newTestUserConfig(), rt, common.GlobalURL)

	bytes, _, err := configFetcher.fetch()
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
	fetch = newFetcher(newTestUserConfig(), rt, common.ProxiesURL).(*fetcher)

	assert.Equal(t, "http://config.getiantem.org/proxies.yaml.gz", fetch.originURL)

	url := common.ProxiesURL

	// Blank flags should mean we use the default
	flags["cloudconfig"] = ""
	fetch = newFetcher(newTestUserConfig(), rt, url).(*fetcher)

	assert.Equal(t, "http://config.getiantem.org/proxies.yaml.gz", fetch.originURL)

	stagingURL := common.ProxiesStagingURL
	flags["staging"] = true
	fetch = newFetcher(newTestUserConfig(), rt, stagingURL).(*fetcher)
	assert.Equal(t, "http://config-staging.getiantem.org/proxies.yaml.gz", fetch.originURL)
}
