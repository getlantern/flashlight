package proxyimpl

import (
	"testing"

	"github.com/getlantern/common/config"
	"github.com/stretchr/testify/assert"
)

func TestPTSettingsNil(t *testing.T) {
	s := &config.ProxyConfig{}
	assert.False(t, ptSettingBool(s, "bool"))
}

func TestPTSettings(t *testing.T) {
	s := &config.ProxyConfig{
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

func TestCiphersFromNames(t *testing.T) {
	assert.Nil(t, ciphersFromNames(nil))
	assert.Nil(t, ciphersFromNames([]string{}))
	assert.Nil(t, ciphersFromNames([]string{"UNKNOWN"}))
	assert.EqualValues(t, []uint16{0x0035, 0x003c}, ciphersFromNames([]string{"TLS_RSA_WITH_AES_256_CBC_SHA", "UNKNOWN", "TLS_RSA_WITH_AES_128_CBC_SHA256"}))
}
