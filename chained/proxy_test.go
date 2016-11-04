package chained

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	obfs4Cert = "8Q2mM+TeX3StSHDrW9sLLE12Q84HK/yYeEMODHJsPYSVinmA2KT+oNngDhbalFSk9bbsOQ"
)

func TestTrusted(t *testing.T) {
	p, _ := CreateProxy(&ChainedServerInfo{Cert: "", PluggableTransport: ""})
	assert.False(t, p.Trusted(), "HTTP proxy should not be trusted")

	p, _ = CreateProxy(&ChainedServerInfo{Cert: obfs4Cert,
		PluggableTransport: "obfs4-tcp",
		PluggableTransportSettings: map[string]string{
			"iat-mode": "0",
		},
	})
	assert.True(t, p.Trusted(), "OBFS4 proxy should be trusted")
	assert.Contains(t, p.Label(), "(trusted)")
}
