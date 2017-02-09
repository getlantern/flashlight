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
			"true":  "true",
			"false": "false",
			"empty": "",
			"2":     "2",
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
}
