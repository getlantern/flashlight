package config

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/embeddedconfig"
)

const (
	applicationVersion = "9999.99.99-dev"
)

func TestValidate(t *testing.T) {
	assert.NoError(t, ClientGroup{}.Validate(), "zero value should be valid")
	assert.NoError(t, ClientGroup{UserFloor: 0.9, UserCeil: 0.98}.Validate(), "valid user range")
	assert.Error(t, ClientGroup{UserFloor: -1.0}.Validate(), "invalid user floor")
	assert.Error(t, ClientGroup{UserFloor: 1.09}.Validate(), "invalid user floor")
	assert.Error(t, ClientGroup{UserFloor: 0.1, UserCeil: 0}.Validate(), "invalid user range")
	assert.Error(t, ClientGroup{Fraction: 1.1}.Validate(), "invalid fraction")
	assert.Error(t, ClientGroup{FreeOnly: true, ProOnly: true}.Validate(), "conflict user status requirements")
	assert.NoError(t, ClientGroup{VersionConstraints: ">3.2.1 <= 9.2.0 "}.Validate(), "compound version constraits")
	assert.NoError(t, ClientGroup{VersionConstraints: "<3.2.1 || >= 9.2.0 "}.Validate(), "compound version constraits")
}

func TestIncludes(t *testing.T) {
	assert.True(t, ClientGroup{}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 0, true, "whatever"), "zero value should include all combinations")
	assert.True(t, ClientGroup{}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, false, "whatever"), "zero value should include all combinations")
	assert.True(t, ClientGroup{UserCeil: 0.12}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, false, "whatever"), "match user range")
	assert.False(t, ClientGroup{UserCeil: 0.11}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, false, "whatever"), "user range does not match")
	assert.False(t, ClientGroup{UserCeil: 0.11}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 0, false, "whatever"), "unknown user ID should not belong to any user range")

	assert.True(t, ClientGroup{FreeOnly: true}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, false, "whatever"), "user status met")
	assert.False(t, ClientGroup{ProOnly: true}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, false, "whatever"), "user status unmet")
	assert.True(t, ClientGroup{ProOnly: true}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "whatever"), "user status met")
	assert.False(t, ClientGroup{FreeOnly: true}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "whatever"), "user status unmet")

	// The default AppName is "Default"
	assert.True(t, ClientGroup{Application: (common.DefaultAppName)}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "whatever"), "application met, case insensitive")
	assert.True(t, ClientGroup{Application: strings.ToUpper(common.DefaultAppName)}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "whatever"), "application met, case insensitive")
	assert.False(t, ClientGroup{Application: "Beam"}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "whatever"), "application unmet, case insensitive")
	assert.False(t, ClientGroup{Application: "beam"}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "whatever"), "application unmet, case insensitive")

	// The client version is 9999.99.99-dev when in development mode
	assert.True(t, ClientGroup{VersionConstraints: "> 5.1.0"}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "whatever"), "version met")
	assert.True(t, ClientGroup{VersionConstraints: "> 5.1.0 < 10000.0.0"}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "whatever"), "version met")
	assert.False(t, ClientGroup{VersionConstraints: "< 5.1.0"}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "whatever"), "version unmet")

	// Platforms tests are likely run
	assert.True(t, ClientGroup{Platforms: "linux,darwin,windows"}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "whatever"), "platform met")
	// Platforms tests are unlikely run
	assert.False(t, ClientGroup{Platforms: "ios,android"}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "whatever"), "platform unmet")

	assert.True(t, ClientGroup{GeoCountries: "ir   , cn"}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "IR"), "country met")
	assert.False(t, ClientGroup{GeoCountries: "us"}.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "IR"), "country unmet")

	// Fraction calculation should be stable
	g := ClientGroup{Fraction: 0.1}
	hits := 0
	for i := 0; i < 1000; i++ {
		if g.Includes(common.Platform, common.DefaultAppName, applicationVersion, 111, true, "whatever") {
			hits++
		}
	}
	if randomFloat >= 0.1 {
		assert.Equal(t, 0, hits)
	} else {
		assert.Equal(t, 1000, hits)
	}
}

func TestUnmarshalFeaturesEnabled(t *testing.T) {
	yml := `
featuresenabled:
  replica:
    - userfloor: 0
      userceil: 0.2
      versionconstraints: ">3.0.0"
      geocountries: us,cn,au,ir
    - versionconstraints: "=9999.99.99-dev"
      proonly: true
`
	gl := NewGlobal()
	if !assert.NoError(t, yaml.Unmarshal([]byte(yml), gl)) {
		return
	}
	assert.True(t, gl.FeatureEnabled(FeatureReplica, common.Platform, common.DefaultAppName, applicationVersion, 111, false, "au"), "met the first group")
	assert.True(t, gl.FeatureEnabled(FeatureReplica, common.Platform, common.DefaultAppName, applicationVersion, 111, true, ""), "met the second group")
	assert.False(t, gl.FeatureEnabled(FeatureReplica, common.Platform, common.DefaultAppName, applicationVersion, 211, false, "au"), "unmet both groups")
	assert.False(t, gl.FeatureEnabled(FeatureReplica, common.Platform, common.DefaultAppName, applicationVersion, 111, false, ""), "unmet both groups")
}

func TestUnmarshalFeatureOptions(t *testing.T) {
	yml := `
featureoptions:
  trafficlog:
    capturebytes: 1
    savebytes: 2
    capturesaveduration: 5m
    reinstall: true
    waittimesincefailedinstall: 24h
    userdenialthreshold: 3
    timebeforedenialreset: 2160h
  pingproxies:
    interval: 1h
`
	gl := NewGlobal()
	require.NoError(t, yaml.Unmarshal([]byte(yml), gl))

	var opts TrafficLogOptions
	require.NoError(t, gl.UnmarshalFeatureOptions(FeatureTrafficLog, &opts))

	require.Equal(t, 1, opts.CaptureBytes)
	require.Equal(t, 2, opts.SaveBytes)
	require.Equal(t, 5*time.Minute, opts.CaptureSaveDuration)
	require.Equal(t, true, opts.Reinstall)
	require.Equal(t, 24*time.Hour, opts.WaitTimeSinceFailedInstall)
	require.Equal(t, 3, opts.UserDenialThreshold)
	require.Equal(t, 2160*time.Hour, opts.TimeBeforeDenialReset)

	var opts2 PingProxiesOptions
	require.NoError(t, gl.UnmarshalFeatureOptions(FeaturePingProxies, &opts2))
	require.Equal(t, time.Hour, opts2.Interval)
}

func TestMatomoEnabled(t *testing.T) {
	gl := globalFromTemplate(t)
	assert.False(t, gl.FeatureEnabled(FeatureMatomo, common.Platform, common.DefaultAppName, applicationVersion, 1, false, "us"), "Matomo is disabled for a low User ID")
	assert.False(t, gl.FeatureEnabled(FeatureMatomo, common.Platform, common.DefaultAppName, applicationVersion, 500, false, "us"), "Matomo is disabled for a high User ID")
}

func TestDetour(t *testing.T) {
	gl := globalFromTemplate(t)
	for _, country := range []string{"cn"} {
		for _, os := range []string{"android", "windows", "darwin", "linux"} {
			assert.False(t, gl.FeatureEnabled(FeatureDetour, os, common.DefaultAppName, applicationVersion, 1, false, country), fmt.Sprintf("detour is disabled for %s in %s", os, country))
		}
	}
}

func TestShortcut(t *testing.T) {
	gl := globalFromTemplate(t)
	for _, country := range []string{"cn", "ir"} {
		for _, os := range []string{"android", "windows", "darwin", "linux"} {
			if country == "cn" {
				assert.True(t, gl.FeatureEnabled(FeatureShortcut, os, common.DefaultAppName, applicationVersion, 1, false, country), fmt.Sprintf("shortcut is enabled for %s in %s", os, country))
			} else {
				assert.False(t, gl.FeatureEnabled(FeatureShortcut, os, common.DefaultAppName, applicationVersion, 1, false, country), fmt.Sprintf("shortcut is disabled for %s in %s", os, country))
			}
		}
	}
}

func TestChatEnabled(t *testing.T) {
	gl := globalFromTemplate(t)
	assert.False(t, gl.FeatureEnabled(FeatureChat, "android", common.DefaultAppName, "7.0.0", 1, false, "ae"), "Chat is disabled in UAE when running 7.0.0")
	assert.False(t, gl.FeatureEnabled(FeatureChat, "android", common.DefaultAppName, "7.0.0", 1, false, "cn"), "Chat is disabled outside UAE when running 7.0.0")
	assert.False(t, gl.FeatureEnabled(FeatureChat, "android", common.DefaultAppName, "6.9.7", 1, false, "ae"), "Chat is disabled in Iran when running 6.9.7")
	assert.False(t, gl.FeatureEnabled(FeatureChat, "android", common.DefaultAppName, "99.0.0", 1, false, "us"), "Chat is disabled in USA when running QA version 99.0.0")
}

func TestReplicaEnabled(t *testing.T) {
	gl := globalFromTemplate(t)
	assert.True(t, gl.FeatureEnabled(FeatureReplica, "android", common.DefaultAppName, "7.0.0", 1, false, "ru"), "Replica is enabled in Russia when running 7.0.0")
	assert.True(t, gl.FeatureEnabled(FeatureReplica, "android", common.DefaultAppName, "7.0.0", 1, false, "ir"), "Replica is enabled in Iran when running 7.0.0")
	assert.False(t, gl.FeatureEnabled(FeatureReplica, "android", common.DefaultAppName, "6.9.7", 1, false, "ru"), "Replica is not enabled in Russia when running 6.9.7")
	assert.True(t, gl.FeatureEnabled(FeatureReplica, "android", common.DefaultAppName, "99.0.0", 1, false, "ru"), "Replica is enabled in Russia when running QA version 99.0.0")
}

func globalFromTemplate(t *testing.T) *Global {
	var w bytes.Buffer
	// We could write into a pipe, but that requires concurrency and we're old-school in tests.
	require.NoError(t, template.Must(template.New("").Parse(embeddedconfig.GlobalTemplate)).Execute(&w, nil))
	g := &Global{}
	require.NoError(t, yaml.Unmarshal(w.Bytes(), g))
	return g
}
