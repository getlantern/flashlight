package chained

import (
	"testing"

	"github.com/getlantern/common/apipb"
	"github.com/stretchr/testify/assert"
)

func TestPTSettingsNil(t *testing.T) {
	s := &apipb.ProxyConfig{}
	assert.False(t, ptSettingBool(s, "bool"))
}

func TestPTSettings(t *testing.T) {
	s := &apipb.ProxyConfig{
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
	}
	assert.True(t, ptSettingBool(s, "true"))
	assert.False(t, ptSettingBool(s, "false"))
	assert.False(t, ptSettingBool(s, "empty"))
	assert.False(t, ptSettingBool(s, "unknown"))
	assert.False(t, ptSettingBool(s, "2"))
	assert.Equal(t, 2, ptSettingInt(s, "2"))
	assert.Equal(t, 0, ptSettingInt(s, "empty"))
	assert.Equal(t, 0, ptSettingInt(s, "unknown"))
	assert.Equal(t, 0, ptSettingInt(s, "false"))

	assert.False(t, ptSettingBool(s, "falsestring"))
	assert.True(t, ptSettingBool(s, "truestring"))
	assert.Equal(t, 2, ptSettingInt(s, "2string"))

	assert.Equal(t, 0, ptSettingInt(s, "badint"))
	assert.False(t, ptSettingBool(s, "badbool"))

	assert.Equal(t, "", ptSetting(s, "empty"))
	assert.Equal(t, "2", ptSetting(s, "2"))
}
