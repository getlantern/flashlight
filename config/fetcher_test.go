package config

import (
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

	configFetcher := newHttpFetcher(newTestUserConfig(), common.GlobalURL)

	bytes, _, err := configFetcher.fetch("testOpName")
	assert.Nil(t, err)
	assert.True(t, len(bytes) > 200)
}
