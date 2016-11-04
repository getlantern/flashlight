package chained

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	obfs4Cert = "8Q2mM+TeX3StSHDrW9sLLE12Q84HK/yYeEMODHJsPYSVinmA2KT+oNngDhbalFSk9bbsOQ"
)

func TestTrusted(t *testing.T) {
	p, _ := CreateProxy(&ChainedServerInfo{Addr: "1.1.1.1", AuthToken: "abcd", Cert: "", PluggableTransport: ""})
	assert.False(t, p.Trusted(), "HTTP proxy should not be trusted")
	assert.NotContains(t, p.Label(), "(trusted)")

	si := &ChainedServerInfo{Addr: "1.1.1.1", AuthToken: "abcd", Cert: obfs4Cert,
		PluggableTransport: "obfs4-tcp",
		PluggableTransportSettings: map[string]string{
			"iat-mode": "0",
		},
	}
	p, _ = CreateProxy(si)
	assert.False(t, p.Trusted(), "OBFS4 proxy should not be trusted by default")
	assert.NotContains(t, p.Label(), "(trusted)")

	si.Trusted = true
	p, _ = CreateProxy(si)
	assert.True(t, p.Trusted(), "OBFS4 proxy should be trusted if explicitly declared")
	assert.Contains(t, p.Label(), "(trusted)")
}
