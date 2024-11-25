package chained

import (
	"strings"
	"testing"

	"github.com/getlantern/common/config"
	"github.com/stretchr/testify/assert"
)

func TestCreateDialersMap(t *testing.T) {
	proxies := map[string]*config.ProxyConfig{
		"proxy1": {
			Addr: "2.2.2.2", AuthToken: "abcd", Cert: "", PluggableTransport: "https",
		},
		"proxy2": {
			Addr: "2.2.2.2", AuthToken: "abcd", Cert: "", PluggableTransport: "https",
		},
	}
	dialers := CreateDialers(tempConfigDir, proxies, newTestUserConfig(), newFronted())
	assert.Equal(t, 2, len(dialers))
	for _, d := range dialers {
		assert.NotNil(t, d)
		assert.True(t, strings.HasPrefix(d.Name(), "proxy"))
	}
}
