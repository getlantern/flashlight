package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/getlantern/yaml"
)

func TestValidate(t *testing.T) {
	assert.NoError(t, ClientGroup{}.Validate(), "zero value should be valid")
	assert.NoError(t, ClientGroup{UserFloor: 0.9, UserCeil: 0.98}.Validate(), "valid user range")
	assert.Error(t, ClientGroup{UserFloor: -1.0}.Validate(), "invalid user floor")
	assert.Error(t, ClientGroup{UserFloor: 1.09}.Validate(), "invalid user floor")
	assert.Error(t, ClientGroup{UserFloor: 0.1, UserCeil: 0}.Validate(), "invalid user range")
	assert.Error(t, ClientGroup{Fraction: 1.1}.Validate(), "invalid fraction")
	assert.Error(t, ClientGroup{FreeOnly: true, ProOnly: true}.Validate(), "conflict user status requirements")
	assert.NoError(t, ClientGroup{VersionConstraints: ">3.2.1 || <= 9.2.0 "}.Validate(), "conflict user status requirements")
}

func TestIncludes(t *testing.T) {
	assert.True(t, ClientGroup{}.Includes("0", true, "whatever"), "zero value should include all combinations")
	assert.True(t, ClientGroup{}.Includes("111", false, "whatever"), "zero value should include all combinations")
	assert.True(t, ClientGroup{UserCeil: 0.12}.Includes("111", false, "whatever"), "match user range")
	assert.False(t, ClientGroup{UserCeil: 0.11}.Includes("111", false, "whatever"), "user range does not match")

	assert.True(t, ClientGroup{FreeOnly: true}.Includes("111", false, "whatever"), "user status met")
	assert.False(t, ClientGroup{ProOnly: true}.Includes("111", false, "whatever"), "user status unmet")
	assert.True(t, ClientGroup{ProOnly: true}.Includes("111", true, "whatever"), "user status met")
	assert.False(t, ClientGroup{FreeOnly: true}.Includes("111", true, "whatever"), "user status unmet")

	// The client version is 9999.99.99-dev when in development mode
	assert.True(t, ClientGroup{VersionConstraints: "> 5.1.0"}.Includes("111", true, "whatever"), "version met")
	assert.False(t, ClientGroup{VersionConstraints: "< 5.1.0"}.Includes("111", true, "whatever"), "version unmet")

	// Platforms tests are likely run
	assert.True(t, ClientGroup{Platforms: "linux,darwin,windows"}.Includes("111", true, "whatever"), "platform met")
	// Platforms tests are unlikely run
	assert.False(t, ClientGroup{Platforms: "ios,android"}.Includes("111", true, "whatever"), "platform unmet")

	assert.True(t, ClientGroup{GeoCountries: "ir   , cn"}.Includes("111", true, "IR"), "country met")
	assert.False(t, ClientGroup{GeoCountries: "us"}.Includes("111", true, "IR"), "country unmet")

	// Fraction calculation should be stable
	g := ClientGroup{Fraction: 0.1}
	hits := 0
	for i := 0; i < 1000; i++ {
		if g.Includes("111", true, "whatever") {
			hits++
		}
	}
	if randomFloat >= 0.1 {
		assert.Equal(t, 0, hits)
	} else {
		assert.Equal(t, 1000, hits)
	}
}

func TestUnmarshalFeatureOptions(t *testing.T) {
	yml := `
featureoptions:
  trafficlog:
    capturebytes: 1
    savebytes: 2
  pingproxies:
    interval: 1h
`
	gl := NewGlobal()
	if !assert.NoError(t, yaml.Unmarshal([]byte(yml), gl)) {
		return
	}
	var opts TrafficLogOptions
	err := gl.UnmarshalFeatureOptions(FeatureTrafficLog, &opts)
	assert.NoError(t, err)
	assert.Equal(t, 1, opts.CaptureBytes)

	var opts2 PingProxiesOptions
	err = gl.UnmarshalFeatureOptions(FeaturePingProxies, &opts2)
	assert.NoError(t, err)
	assert.Equal(t, time.Hour, opts2.Interval)
}
