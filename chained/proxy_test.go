package chained

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/common/config"
)

var (
	obfs4Cert = "8Q2mM+TeX3StSHDrW9sLLE12Q84HK/yYeEMODHJsPYSVinmA2KT+oNngDhbalFSk9bbsOQ"
)

func TestTrusted(t *testing.T) {
	d, _ := CreateDialer(tempConfigDir, "test-proxy",
		&config.ProxyConfig{Addr: "1.1.1.1", AuthToken: "abcd", Cert: "", PluggableTransport: ""},
		newTestUserConfig(), false)
	assert.False(t, d.Trusted(), "HTTP proxy should not be trusted")
	assert.NotContains(t, d.JustifiedLabel(), trustedSuffix)

	si := &config.ProxyConfig{
		Addr: "1.1.1.1", AuthToken: "abcd", Cert: obfs4Cert,
		PluggableTransport: "obfs4",
		PluggableTransportSettings: map[string]string{
			"iat-mode": "0",
		},
	}
	d, _ = CreateDialer(tempConfigDir, "test-proxy", si, newTestUserConfig(), false)
	assert.False(t, d.Trusted(), "OBFS4 proxy should not be trusted by default")
	assert.NotContains(t, d.JustifiedLabel(), trustedSuffix)

	si.Trusted = true
	d, _ = CreateDialer(tempConfigDir, "test-proxy", si, newTestUserConfig(), false)
	assert.True(t, d.Trusted(), "OBFS4 proxy should be trusted if explicitly declared")
	assert.Contains(t, d.JustifiedLabel(), trustedSuffix)
}

func TestCreateDialersMap(t *testing.T) {
	proxies := map[string]*config.ProxyConfig{
		"proxy1": &config.ProxyConfig{
			Addr: "2.2.2.2", AuthToken: "abcd", Cert: "", PluggableTransport: "https",
		},
		"proxy2": &config.ProxyConfig{
			Addr: "2.2.2.2", AuthToken: "abcd", Cert: "", PluggableTransport: "https",
		},
	}
	dialers := CreateDialers(tempConfigDir, proxies, newTestUserConfig())
	assert.Equal(t, 2, len(dialers))
	for _, d := range dialers {
		assert.NotNil(t, d)
		assert.True(t, strings.HasPrefix(d.Name(), "proxy"))
	}
}
