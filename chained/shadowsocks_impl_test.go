// +build !iosapp

package chained

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/shadowsocks/go-shadowsocks2/socks"
	"github.com/stretchr/testify/assert"
)

func TestGenerateUpstream(t *testing.T) {
	info := &ChainedServerInfo{
		PluggableTransportSettings: map[string]string{
			"shadowsocks_secret":   "foo",
			"shadowsocks_cipher":   "chacha20-ietf-poly1305",
			"shadowsocks_upstream": "test",
		},
	}

	prx, err := newShadowsocksImpl("test-0", "127.0.0.1:4444", info, nil)
	if !assert.Nil(t, err, "Failed to create proxy impl: %v", err) {
		return
	}
	ng := prx.(*shadowsocksImpl)
	if !assert.NotNil(t, ng, "proxy impl had unexpected type") {
		return
	}

	runs := 10000
	for i := 0; i < runs; i++ {
		up := ng.generateUpstream()
		host, _, err := net.SplitHostPort(up)
		assert.Nil(t, err, "result address did not have a host and port")
		assert.True(t, strings.HasSuffix(host, ".test"), "Unexpected suffix %s", up)
		addr := socks.ParseAddr(up)
		assert.NotNil(t, addr, "Unable to parse upstream address %s", up)
		_, err = socks.ReadAddr(bytes.NewReader(addr))
		assert.Nil(t, err, "Failed to read upstream as an addr %s", up)
	}
}
