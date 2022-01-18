package config

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/getlantern/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getlantern/flashlight/common"
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
	assert.True(t, ClientGroup{}.Includes(common.DefaultAppName, 0, true, "whatever"), "zero value should include all combinations")
	assert.True(t, ClientGroup{}.Includes(common.DefaultAppName, 111, false, "whatever"), "zero value should include all combinations")
	assert.True(t, ClientGroup{UserCeil: 0.12}.Includes(common.DefaultAppName, 111, false, "whatever"), "match user range")
	assert.False(t, ClientGroup{UserCeil: 0.11}.Includes(common.DefaultAppName, 111, false, "whatever"), "user range does not match")
	assert.False(t, ClientGroup{UserCeil: 0.11}.Includes(common.DefaultAppName, 0, false, "whatever"), "unknown user ID should not belong to any user range")

	assert.True(t, ClientGroup{FreeOnly: true}.Includes(common.DefaultAppName, 111, false, "whatever"), "user status met")
	assert.False(t, ClientGroup{ProOnly: true}.Includes(common.DefaultAppName, 111, false, "whatever"), "user status unmet")
	assert.True(t, ClientGroup{ProOnly: true}.Includes(common.DefaultAppName, 111, true, "whatever"), "user status met")
	assert.False(t, ClientGroup{FreeOnly: true}.Includes(common.DefaultAppName, 111, true, "whatever"), "user status unmet")

	// The default AppName is "Default"
	assert.True(t, ClientGroup{Application: (common.DefaultAppName)}.Includes(common.DefaultAppName, 111, true, "whatever"), "application met, case insensitive")
	assert.True(t, ClientGroup{Application: strings.ToUpper(common.DefaultAppName)}.Includes(common.DefaultAppName, 111, true, "whatever"), "application met, case insensitive")
	assert.False(t, ClientGroup{Application: "Beam"}.Includes(common.DefaultAppName, 111, true, "whatever"), "application unmet, case insensitive")
	assert.False(t, ClientGroup{Application: "beam"}.Includes(common.DefaultAppName, 111, true, "whatever"), "application unmet, case insensitive")

	// The client version is 9999.99.99-dev when in development mode
	assert.True(t, ClientGroup{VersionConstraints: "> 5.1.0"}.Includes(common.DefaultAppName, 111, true, "whatever"), "version met")
	assert.True(t, ClientGroup{VersionConstraints: "> 5.1.0 < 10000.0.0"}.Includes(common.DefaultAppName, 111, true, "whatever"), "version met")
	assert.False(t, ClientGroup{VersionConstraints: "< 5.1.0"}.Includes(common.DefaultAppName, 111, true, "whatever"), "version unmet")

	// Platforms tests are likely run
	assert.True(t, ClientGroup{Platforms: "linux,darwin,windows"}.Includes(common.DefaultAppName, 111, true, "whatever"), "platform met")
	// Platforms tests are unlikely run
	assert.False(t, ClientGroup{Platforms: "ios,android"}.Includes(common.DefaultAppName, 111, true, "whatever"), "platform unmet")

	assert.True(t, ClientGroup{GeoCountries: "ir   , cn"}.Includes(common.DefaultAppName, 111, true, "IR"), "country met")
	assert.False(t, ClientGroup{GeoCountries: "us"}.Includes(common.DefaultAppName, 111, true, "IR"), "country unmet")

	// Fraction calculation should be stable
	g := ClientGroup{Fraction: 0.1}
	hits := 0
	for i := 0; i < 1000; i++ {
		if g.Includes(common.DefaultAppName, 111, true, "whatever") {
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
	assert.True(t, gl.FeatureEnabled(FeatureReplica, common.DefaultAppName, 111, false, "au"), "met the first group")
	assert.True(t, gl.FeatureEnabled(FeatureReplica, common.DefaultAppName, 111, true, ""), "met the second group")
	assert.False(t, gl.FeatureEnabled(FeatureReplica, common.DefaultAppName, 211, false, "au"), "unmet both groups")
	assert.False(t, gl.FeatureEnabled(FeatureReplica, common.DefaultAppName, 111, false, ""), "unmet both groups")
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

func TestUnmarshalAnalyticsOptions(t *testing.T) {
	yml := `
featureoptions:
  analytics:
    providers:
      ga: 
        endpoint: "https://ssl.google-analytics.com/collect"
        samplerate: 1.0
        config:
          k1: 2
          k2: 3
      matomo: 
        samplerate: 0.1
        config:
          idsite: 1
          token_auth: "418290ccds0d01"
`
	gl := NewGlobal()
	require.NoError(t, yaml.Unmarshal([]byte(yml), gl))

	var opts AnalyticsOptions
	require.NoError(t, gl.UnmarshalFeatureOptions(FeatureAnalytics, &opts))
	log.Debugf("%+v", opts)

	mat := opts.GetProvider(MATOMO)
	ga := opts.GetProvider(GA)
	require.Equal(t, float32(0.1), mat.SampleRate)
	require.Equal(t, 2, ga.Config["k1"])
	require.Equal(t, "https://ssl.google-analytics.com/collect", ga.Endpoint)

	require.Equal(t, 1, mat.Config["idsite"])
	require.Nil(t, mat.Config["k1"])
}

func TestReplicaByCountry(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	fos := getReplicaOptionsRoot(require)
	assert.Contains(fos.ByCountry, "RU")
	assert.NotContains(fos.ByCountry, "AU")
	assert.NotEmpty(fos.ByCountry)
	globalTrackers := fos.Trackers
	assert.NotEmpty(globalTrackers)
	// Check the countries pull in the trackers using the anchor. Just change this if they stop
	// using the same trackers. I really don't want this to break out the gate is all.
	assert.Equal(fos.ByCountry["CN"].Trackers, globalTrackers)
	assert.Equal(fos.ByCountry["RU"].Trackers, globalTrackers)
	assert.Equal(fos.ByCountry["IR"].Trackers, globalTrackers)
}

func getReplicaOptionsRoot(require *require.Assertions) (fos ReplicaOptionsRoot) {
	var w bytes.Buffer
	// We could write into a pipe, but that requires concurrency and we're old-school in tests.
	require.NoError(template.Must(template.New("").Parse(replicaGlobalConfig)).Execute(&w, nil))
	var g Global
	require.NoError(yaml.Unmarshal(w.Bytes(), &g))
	require.NoError(g.UnmarshalFeatureOptions(FeatureReplica, &fos))
	return
}

func TestReplicaProxying(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	fos := getReplicaOptionsRoot(require)
	numInfohashes := len(fos.ProxyAnnounceTargets)
	// The default is to announce as a proxy.
	assert.True(numInfohashes > 0)
	// The default is not to look for proxies
	assert.Empty(fos.ProxyPeerInfoHashes)
	// Iran looks for peers from the default countries.
	assert.Len(fos.ByCountry["IR"].ProxyPeerInfoHashes, numInfohashes)
}

var replicaGlobalConfig = `
featuresenabled:
  replica:
    # DE, AU and CN are mainly there since members of Replica team live there
    # XXX 13-01-22 @soltzen: Adding CA because of the NCC auditor testing. This can be removed afterwards
    - label: replica-desktop
      geocountries: cn,au,ir,de,ru,by,ca
      platforms: darwin,windows,linux
      # this filter works only for Lantern 6.1+
      application: lantern
    # US, DE, AU and GR are mainly there since members of Replica team live there
    - label: replica-android
      geocountries: ir,us,au,de,gr,ca
      platforms: android
    - label: all-beam
      application: beam
      versionconstraints: "<4.0.0"
featureoptions:
  replica:
    # Uses ISO 3166 country codes
    # https://en.wikipedia.org/wiki/List_of_ISO_3166_country_codes
    # Also, quotes are necessary around key names, else Norway (NO) will be
    # interpreted as a boolean

    # These are the default options

    metadatabaseurls: &AllReplicaBaseUrls
    - https://s3.ap-southeast-1.amazonaws.com/getlantern-replica/
    - https://s3-ap-southeast-1.amazonaws.com/getlantern-replica/
    - https://s3.eu-central-1.amazonaws.com/getlantern-replica-frankfurt/
    - https://s3-eu-central-1.amazonaws.com/getlantern-replica-frankfurt/
    - https://d3mm73d1kmj7zd.cloudfront.net/
    replicarustendpoint: &GlobalReplicaRust https://replica-search.lantern.io/
    staticpeeraddrs: []
    trackers: &GlobalTrackers
    - https://tracker.gbitt.info:443/announce
    - http://tracker.opentrackr.org:1337/announce
    - udp://tracker.leechers-paradise.org:6969/announce
    webseedbaseurls: *AllReplicaBaseUrls
    proxyannouncetargets: &GlobalProxyInfohashes
    - 94c3fe9ead4625e0529c334bbb90568accb35ce3
    - 77c89c352dcca36846c77541d9e2c6b4aa944790
    - dbea91bb2d2953dc8c46272eb32cb4781ec588c5

    # These are for compatibility with clients that don't load all options per-country.
    replicarustdefaultendpoint: *GlobalReplicaRust
    replicarustendpoints:
      "RU": &FrankfurtReplicaRust https://replica-search-aws.lantern.io/
      "IR": *FrankfurtReplicaRust
      "DE": *FrankfurtReplicaRust

    # Here follows options per-country

    "IR":
      proxypeerinfohashes: *GlobalProxyInfohashes
      metadatabaseurls: *AllReplicaBaseUrls
      replicarustendpoint: *FrankfurtReplicaRust
      staticpeeraddrs: [&IranReplicaPeer "81.12.39.55:42069"]
      trackers: *GlobalTrackers
      webseedbaseurls: *AllReplicaBaseUrls
    "CN":
      proxypeerinfohashes: *GlobalProxyInfohashes
      metadatabaseurls:
      - https://s3.ap-southeast-1.amazonaws.com/getlantern-replica/
      - https://s3-ap-southeast-1.amazonaws.com/getlantern-replica/
      replicarustendpoint: *GlobalReplicaRust
      staticpeeraddrs: []
      trackers: *GlobalTrackers
      webseedbaseurls:
      - https://s3.ap-southeast-1.amazonaws.com/getlantern-replica/
      - https://s3-ap-southeast-1.amazonaws.com/getlantern-replica/
    "RU":
      proxypeerinfohashes: *GlobalProxyInfohashes
      metadatabaseurls: *AllReplicaBaseUrls
      replicarustendpoint: *FrankfurtReplicaRust
      staticpeeraddrs: [*IranReplicaPeer]
      trackers: *GlobalTrackers
      webseedbaseurls: *AllReplicaBaseUrls

# This is the original global config, still here for compatibility with very old clients.
replica:
  webseedbaseurls: *AllReplicaBaseUrls
  trackers: *GlobalTrackers
  staticpeeraddrs: []
  metadatabaseurls: *AllReplicaBaseUrls
  replicaserviceendpoint: *GlobalReplicaRust
`