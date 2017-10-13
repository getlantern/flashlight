package chained

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPTSettingsNil(t *testing.T) {
	s := &ChainedServerInfo{}
	assert.False(t, s.ptSettingBool("bool"))
}

func TestPTSettings(t *testing.T) {
	s := &ChainedServerInfo{
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
	assert.True(t, s.ptSettingBool("true"))
	assert.False(t, s.ptSettingBool("false"))
	assert.False(t, s.ptSettingBool("empty"))
	assert.False(t, s.ptSettingBool("unknown"))
	assert.False(t, s.ptSettingBool("2"))
	assert.Equal(t, 2, s.ptSettingInt("2"))
	assert.Equal(t, 0, s.ptSettingInt("empty"))
	assert.Equal(t, 0, s.ptSettingInt("unknown"))
	assert.Equal(t, 0, s.ptSettingInt("false"))

	assert.False(t, s.ptSettingBool("falsestring"))
	assert.True(t, s.ptSettingBool("truestring"))
	assert.Equal(t, 2, s.ptSettingInt("2string"))

	assert.Equal(t, 0, s.ptSettingInt("badint"))
	assert.False(t, s.ptSettingBool("badbool"))

	assert.Equal(t, "", s.ptSetting("empty"))
	assert.Equal(t, "2", s.ptSetting("2"))
}
