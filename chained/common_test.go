package chained

import (
	"testing"

	"github.com/getlantern/common/config"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestCopyConfigs(t *testing.T) {
	proxies := map[string]*config.ProxyConfig{
		"pc1": {
			PluggableTransportSettings: map[string]string{
				"true":        "true",
				"false":       "false",
				"empty":       "",
				"2":           "2",
				"falsestring": "false",
				"truestring":  "true",
				"2string":     "2",

				"badint":  "notint",
				"badbool": "notbool",
			},
		},
		"pc2": {
			AuthToken: "token",
			PluggableTransportSettings: map[string]string{
				"true":        "true",
				"false":       "false",
				"empty":       "",
				"2":           "2",
				"falsestring": "false",
				"truestring":  "true",
				"2string":     "2",

				"badint":  "notint",
				"badbool": "notbool",
			},
		},
	}
	assert.True(t, proto.Equal(proxies["pc1"], CopyConfigs(proxies)["pc1"]))
	assert.True(t, proto.Equal(proxies["pc2"], CopyConfigs(proxies)["pc2"]))
}
