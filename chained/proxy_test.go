package chained

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	obfs4Cert = "8Q2mM+TeX3StSHDrW9sLLE12Q84HK/yYeEMODHJsPYSVinmA2KT+oNngDhbalFSk9bbsOQ"
)

func TestTrusted(t *testing.T) {
	d, _ := CreateDialer(tempConfigDir, "test-proxy", &ChainedServerInfo{Addr: "1.1.1.1", AuthToken: "abcd", Cert: "", PluggableTransport: ""}, newTestUserConfig())
	assert.False(t, d.Trusted(), "HTTP proxy should not be trusted")
	assert.NotContains(t, d.JustifiedLabel(), trustedSuffix)

	si := &ChainedServerInfo{Addr: "1.1.1.1", AuthToken: "abcd", Cert: obfs4Cert,
		PluggableTransport: "obfs4",
		PluggableTransportSettings: map[string]string{
			"iat-mode": "0",
		},
	}
	d, _ = CreateDialer(tempConfigDir, "test-proxy", si, newTestUserConfig())
	assert.False(t, d.Trusted(), "OBFS4 proxy should not be trusted by default")
	assert.NotContains(t, d.JustifiedLabel(), trustedSuffix)

	si.Trusted = true
	d, _ = CreateDialer(tempConfigDir, "test-proxy", si, newTestUserConfig())
	assert.True(t, d.Trusted(), "OBFS4 proxy should be trusted if explicitly declared")
	assert.Contains(t, d.JustifiedLabel(), trustedSuffix)
}
