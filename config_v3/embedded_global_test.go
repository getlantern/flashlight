package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/v7/embeddedconfig"
)

func TestEmbeddedGlobal(t *testing.T) {

	globalFunc := newGlobalUnmarshaler(make(map[string]interface{}))

	global, err := globalFunc(embeddedconfig.Global)
	assert.NoError(t, err)

	gl := global.(*Global)
	akamai := gl.Client.Fronted.Providers["akamai"]
	cloudfront := gl.Client.Fronted.Providers["cloudfront"]
	assert.True(t, len(akamai.Masquerades) > 20, "embedded global config does not contain enough akamai masquerades: ")
	assert.True(t, len(cloudfront.Masquerades) > 20, "embedded global config does not contain enough cloudfront masquerades")
	assert.Containsf(t, cloudfront.HostAliases, "replica-search.lantern.io", "embedded global config does not contain replica-search cloudfront fronted provider")
	assert.Containsf(t, akamai.HostAliases, "replica-search.lantern.io", "embedded global config does not contain replica-search akamai fronted provider")
}
