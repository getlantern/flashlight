package config

import (
	"testing"

	"github.com/getlantern/flashlight/config/generated"

	"github.com/stretchr/testify/assert"
)

func TestGlobal(t *testing.T) {

	globalFunc := newGlobalUnmarshaler(make(map[string]interface{}))

	global, err := globalFunc(generated.GlobalConfig)
	assert.NoError(t, err)

	gl := global.(*Global)
	assert.True(t, len(gl.Client.Fronted.Providers["akamai"].Masquerades) > 20)
	assert.True(t, len(gl.Client.Fronted.Providers["cloudfront"].Masquerades) > 20)
	assert.Containsf(t, gl.Client.Fronted.Providers["cloudfront"].HostAliases, "replica-search.lantern.io", "embedded global config does not contain replica-search cloudfront fronted provider")
	assert.Containsf(t, gl.Client.Fronted.Providers["akamai"].HostAliases, "replica-search.lantern.io", "embedded global config does not contain replica-search akamai fronted provider")
	assert.Len(t, gl.Replica.Trackers, 3)

}
