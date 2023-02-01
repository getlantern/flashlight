package config

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/getlantern/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/embeddedconfig"
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
	assert.True(t, ClientGroup{}.Includes(common.Platform, common.DefaultAppName, common.Version, 0, true, "whatever"), "zero value should include all combinations")
	assert.True(t, ClientGroup{}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, false, "whatever"), "zero value should include all combinations")
	assert.True(t, ClientGroup{UserCeil: 0.12}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, false, "whatever"), "match user range")
	assert.False(t, ClientGroup{UserCeil: 0.11}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, false, "whatever"), "user range does not match")
	assert.False(t, ClientGroup{UserCeil: 0.11}.Includes(common.Platform, common.DefaultAppName, common.Version, 0, false, "whatever"), "unknown user ID should not belong to any user range")

	assert.True(t, ClientGroup{FreeOnly: true}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, false, "whatever"), "user status met")
	assert.False(t, ClientGroup{ProOnly: true}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, false, "whatever"), "user status unmet")
	assert.True(t, ClientGroup{ProOnly: true}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "whatever"), "user status met")
	assert.False(t, ClientGroup{FreeOnly: true}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "whatever"), "user status unmet")

	// The default AppName is "Default"
	assert.True(t, ClientGroup{Application: (common.DefaultAppName)}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "whatever"), "application met, case insensitive")
	assert.True(t, ClientGroup{Application: strings.ToUpper(common.DefaultAppName)}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "whatever"), "application met, case insensitive")
	assert.False(t, ClientGroup{Application: "Beam"}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "whatever"), "application unmet, case insensitive")
	assert.False(t, ClientGroup{Application: "beam"}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "whatever"), "application unmet, case insensitive")

	// The client version is 9999.99.99-dev when in development mode
	assert.True(t, ClientGroup{VersionConstraints: "> 5.1.0"}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "whatever"), "version met")
	assert.True(t, ClientGroup{VersionConstraints: "> 5.1.0 < 10000.0.0"}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "whatever"), "version met")
	assert.False(t, ClientGroup{VersionConstraints: "< 5.1.0"}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "whatever"), "version unmet")

	// Platforms tests are likely run
	assert.True(t, ClientGroup{Platforms: "linux,darwin,windows"}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "whatever"), "platform met")
	// Platforms tests are unlikely run
	assert.False(t, ClientGroup{Platforms: "ios,android"}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "whatever"), "platform unmet")

	assert.True(t, ClientGroup{GeoCountries: "ir   , cn"}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "IR"), "country met")
	assert.False(t, ClientGroup{GeoCountries: "us"}.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "IR"), "country unmet")

	// Fraction calculation should be stable
	g := ClientGroup{Fraction: 0.1}
	hits := 0
	for i := 0; i < 1000; i++ {
		if g.Includes(common.Platform, common.DefaultAppName, common.Version, 111, true, "whatever") {
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
	assert.True(t, gl.FeatureEnabled(FeatureReplica, common.Platform, common.DefaultAppName, common.Version, 111, false, "au"), "met the first group")
	assert.True(t, gl.FeatureEnabled(FeatureReplica, common.Platform, common.DefaultAppName, common.Version, 111, true, ""), "met the second group")
	assert.False(t, gl.FeatureEnabled(FeatureReplica, common.Platform, common.DefaultAppName, common.Version, 211, false, "au"), "unmet both groups")
	assert.False(t, gl.FeatureEnabled(FeatureReplica, common.Platform, common.DefaultAppName, common.Version, 111, false, ""), "unmet both groups")
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
}

func TestMatomoEnabled(t *testing.T) {
	gl := globalFromTemplate(t)
	assert.True(t, gl.FeatureEnabled(FeatureMatomo, common.Platform, common.DefaultAppName, common.Version, 1, false, "us"), "Matomo is enabled for a low User ID")
	assert.True(t, gl.FeatureEnabled(FeatureMatomo, common.Platform, common.DefaultAppName, common.Version, 500, false, "us"), "Matomo is enabled for a high User ID")
}

func TestDetour(t *testing.T) {
	gl := globalFromTemplate(t)
	for _, country := range []string{"cn"} {
		for _, os := range []string{"android", "windows", "darwin", "linux"} {
			assert.False(t, gl.FeatureEnabled(FeatureDetour, os, common.DefaultAppName, common.Version, 1, false, country), fmt.Sprintf("detour is disabled for %s in %s", os, country))
		}
	}
}

func TestShortcut(t *testing.T) {
	gl := globalFromTemplate(t)
	for _, country := range []string{"cn", "ir"} {
		for _, os := range []string{"android", "windows", "darwin", "linux"} {
			if country == "cn" {
				assert.True(t, gl.FeatureEnabled(FeatureShortcut, os, common.DefaultAppName, common.Version, 1, false, country), fmt.Sprintf("shortcut is enabled for %s in %s", os, country))
			} else {
				assert.False(t, gl.FeatureEnabled(FeatureShortcut, os, common.DefaultAppName, common.Version, 1, false, country), fmt.Sprintf("shortcut is disabled for %s in %s", os, country))
			}
		}
	}
}

func TestReplicaByCountry(t *testing.T) {
	assert := assert.New(t)
	fos := getReplicaOptionsRoot(t)
	assert.Contains(fos.ByCountry, "RU")
	assert.NotContains(fos.ByCountry, "AU")
	assert.NotEmpty(fos.ByCountry)
	globalTrackers := fos.Trackers
	assert.NotEmpty(globalTrackers)
	// Check the countries pull in the trackers using the anchor. Just change this if they stop
	// using the same trackers. I really don't want this to break out the gate is all.
	assert.NotEmpty(fos.ByCountry["RU"].Trackers)
	assert.Equal(fos.ByCountry["IR"].Trackers, globalTrackers)
}

func TestP2PEnabledAndFeatures(t *testing.T) {
	// TODO <04-07-2022, soltzen> This part of the test, along with most other
	// "enabled" tests in this file, are really weak: they mainly test isolated
	// cases when they should return the **all** acceptable states and assert
	// they all work. For example, the test below checks that P2P feature is
	// **only** enabled in version >= 99.0.0. It does this by asserting that
	// version == 99.0.0 works and asserting that 7.0.0 doesn't work, while
	// ignoring the other infinite number of versions that might or might not
	// work.
	//
	// A better test would be to get a list of constraints from an enabled
	// feature and assert those are the same as what's expected.
	gl := globalFromTemplate(t)
	var fpOpts P2PFreePeerOptions
	require.NoError(t, gl.UnmarshalFeatureOptions(FeatureP2PFreePeer, &fpOpts))
	require.Contains(t, fpOpts.RegistrarEndpoint, "p2pregistrar")
	require.NotEmpty(t, fpOpts.DomainWhitelist)

	var cpOpts P2PCensoredPeerOptions
	require.NoError(t, gl.UnmarshalFeatureOptions(FeatureP2PCensoredPeer, &cpOpts))
	require.NotEqual(t, 0, len(cpOpts.Bep44TargetsAndSalts))
}

func TestChatEnabled(t *testing.T) {
	gl := globalFromTemplate(t)
	assert.False(t, gl.FeatureEnabled(FeatureChat, "android", common.DefaultAppName, "7.0.0", 1, false, "ae"), "Chat is disabled in UAE when running 7.0.0")
	assert.False(t, gl.FeatureEnabled(FeatureChat, "android", common.DefaultAppName, "7.0.0", 1, false, "ir"), "Chat is disabled in Iran when running 7.0.0")
	assert.False(t, gl.FeatureEnabled(FeatureChat, "android", common.DefaultAppName, "7.0.0", 1, false, "cn"), "Chat is disabled in CN when running 7.0.0")
	assert.False(t, gl.FeatureEnabled(FeatureChat, "android", common.DefaultAppName, "6.9.7", 1, false, "ae"), "Chat is disabled in Iran when running 6.9.7")
	assert.False(t, gl.FeatureEnabled(FeatureChat, "android", common.DefaultAppName, "7.1.0", 1, false, "ae"), "Chat is disabled in China when running 7.1")
	assert.False(t, gl.FeatureEnabled(FeatureChat, "android", common.DefaultAppName, "99.0.0", 1, false, "us"), "Chat is disabled in USA when running QA version 99.0.0")
}

func TestReplicaEnabled(t *testing.T) {
	gl := globalFromTemplate(t)
	assert.True(t, gl.FeatureEnabled(FeatureReplica, "android", common.DefaultAppName, "6.9.11", 1, false, "ru"), "Replica is enabled in Russia when running 6.9.11")
	assert.True(t, gl.FeatureEnabled(FeatureReplica, "android", common.DefaultAppName, "7.0.0", 1, false, "ir"), "Replica is enabled in Iran when running 6.9.11")
	assert.False(t, gl.FeatureEnabled(FeatureReplica, "android", common.DefaultAppName, "7.0.0", 1, false, "us"), "Replica is not enabled in USA when running 7.0.0")
	assert.False(t, gl.FeatureEnabled(FeatureReplica, "android", common.DefaultAppName, "6.9.10", 1, false, "ru"), "Replica is not enabled in Russia when running 6.9.10")
	assert.False(t, gl.FeatureEnabled(FeatureReplica, "android", common.DefaultAppName, "6.9.11", 1, false, "us"), "Replica is not enabled in USA when running 6.9.11")
	assert.True(t, gl.FeatureEnabled(FeatureReplica, "android", common.DefaultAppName, "99.0.0", 1, false, "us"), "Replica is enabled in USA when running QA version 99.0.0")
}

func TestOtelEnabled(t *testing.T) {
	gl := globalFromTemplate(t)
	assert.False(t, gl.FeatureEnabled(FeatureOtel, "android", common.DefaultAppName, "7.0.0", 1, false, "ae"), "Otel is enabled for low user in UAE")
	assert.False(t, gl.FeatureEnabled(FeatureOtel, "android", common.DefaultAppName, "7.0.0", 500, false, "ae"), "Otel is enabled for high user in UAE")
}

func TestBroflakeEnabled(t *testing.T) {
	yml := `
featureoptions:
  broflake:
    discovery_server: https://discovery.broflake.org
    nat_timeout: 5
`
	gl := NewGlobal()
	require.NoError(t, yaml.Unmarshal([]byte(yml), gl))

	var opts BroflakeOptions
	require.NoError(t, gl.UnmarshalFeatureOptions(FeatureBroflake, &opts))
	assert.Equal(t, "https://discovery.broflake.org", opts.DiscoverySrv)
	assert.Equal(t, time.Duration(5), opts.NATFailTimeout)
}

func getReplicaOptionsRoot(t *testing.T) (fos ReplicaOptionsRoot) {
	g := globalFromTemplate(t)
	require.NoError(t, g.UnmarshalFeatureOptions(FeatureReplica, &fos))
	return
}

func globalFromTemplate(t *testing.T) *Global {
	var w bytes.Buffer
	// We could write into a pipe, but that requires concurrency and we're old-school in tests.
	require.NoError(t, template.Must(template.New("").Parse(embeddedconfig.GlobalTemplate)).Execute(&w, nil))
	g := &Global{}
	require.NoError(t, yaml.Unmarshal(w.Bytes(), g))
	return g
}
