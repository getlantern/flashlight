package config

import (
	"fmt"
	"math/rand"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/mitchellh/mapstructure"

	"github.com/getlantern/errors"

	globalConfig "github.com/getlantern/flashlight/v7/config/global"
)

const (
	FeatureAuth                 = "auth"
	FeatureProxyBench           = "proxybench"
	FeatureTrafficLog           = "trafficlog"
	FeatureNoBorda              = "noborda"
	FeatureProbeProxies         = "probeproxies"
	FeatureShortcut             = "shortcut"
	FeatureDetour               = "detour"
	FeatureNoHTTPSEverywhere    = "nohttpseverywhere"
	FeatureReplica              = globalConfig.FeatureReplica
	FeatureProxyWhitelistedOnly = "proxywhitelistedonly"
	FeatureTrackYouTube         = "trackyoutube"
	FeatureGoogleSearchAds      = "googlesearchads"
	FeatureChat                 = "chat"
	FeatureOtel                 = "otel"
	FeatureInterstitialAds      = "interstitialads"
	FeatureTapsellAds           = "tapsellads"
)

var (
	// to have stable calculation of fraction until the client restarts.
	randomFloat = rand.Float64()

	errAbsentOption    = globalConfig.ErrFeatureOptionAbsent
	errMalformedOption = errors.New("malformed option")
)

type FeatureOptions = globalConfig.FeatureOptions

type GoogleSearchAdsOptions struct {
	Pattern     string                 `mapstructure:"pattern"`
	BlockFormat string                 `mapstructure:"block_format"`
	AdFormat    string                 `mapstructure:"ad_format"`
	Partners    map[string][]PartnerAd `mapstructure:"partners"`
}

type PartnerAd struct {
	Name        string
	URL         string
	Campaign    string
	Description string
	Keywords    []*regexp.Regexp
	Probability float32
}

func (o *GoogleSearchAdsOptions) FromMap(m map[string]interface{}) error {
	// since keywords can be regexp and we don't want to compile them each time we compare, define a custom decode hook
	// that will convert string to regexp and error out on syntax issues
	config := &mapstructure.DecoderConfig{
		DecodeHook: func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
			if t != reflect.TypeOf(regexp.Regexp{}) {
				return data, nil
			}
			r, err := regexp.Compile(fmt.Sprintf("%v", data))
			if err != nil {
				return nil, err
			}
			return r, nil
		},
		Result: o,
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(m)
}

// TrafficLogOptions represents options for github.com/getlantern/trafficlog-flashlight.
type TrafficLogOptions struct {
	// Size of the traffic log's packet buffers (if enabled).
	CaptureBytes int
	SaveBytes    int

	// How far back to go when attaching packets to an issue report.
	CaptureSaveDuration time.Duration

	// Whether to overwrite the traffic log binary. This may result in users being re-prompted for
	// their passwords. The binary will never be overwritten if the existing binary matches the
	// embedded version.
	Reinstall bool

	// The minimum amount of time to wait before re-prompting the user since the last time we failed
	// to install the traffic log. The most likely reason for a failed install is denial of
	// permission by the user. A value of 0 means we never re-attempt installation.
	WaitTimeSinceFailedInstall time.Duration

	// The number of times installation can fail before we give up on this client. A value of zero
	// is equivalent to a value of one.
	FailuresThreshold int

	// After this amount of time has elapsed, the failure count is reset and a user may be
	// re-prompted to install the traffic log.
	TimeBeforeFailureReset time.Duration

	// The number of times a user must deny permission for the traffic log before we stop asking. A
	// value of zero is equivalent to a value of one.
	UserDenialThreshold int

	// After this amount of time has elapsed, the user denial count is reset and a user may be
	// re-prompted to install the traffic log.
	TimeBeforeDenialReset time.Duration
}

func (o *TrafficLogOptions) FromMap(m map[string]interface{}) error {
	var err error
	o.CaptureBytes, err = somethingFromMap[int](m, "capturebytes")
	if err != nil {
		return errors.New("error unmarshaling 'capturebytes': %v", err)
	}
	o.SaveBytes, err = somethingFromMap[int](m, "savebytes")
	if err != nil {
		return errors.New("error unmarshaling 'savebytes': %v", err)
	}
	o.CaptureSaveDuration, err = durationFromMap(m, "capturesaveduration")
	if err != nil {
		return errors.New("error unmarshaling 'capturesaveduration': %v", err)
	}
	o.Reinstall, err = somethingFromMap[bool](m, "reinstall")
	if err != nil {
		return errors.New("error unmarshaling 'reinstall': %v", err)
	}
	o.WaitTimeSinceFailedInstall, err = durationFromMap(m, "waittimesincefailedinstall")
	if err != nil {
		return errors.New("error unmarshaling 'waittimesincefailedinstall': %v", err)
	}
	o.UserDenialThreshold, err = somethingFromMap[int](m, "userdenialthreshold")
	if err != nil {
		return errors.New("error unmarshaling 'userdenialthreshold': %v", err)
	}
	o.TimeBeforeDenialReset, err = durationFromMap(m, "timebeforedenialreset")
	if err != nil {
		return errors.New("error unmarshaling 'timebeforedenialreset': %v", err)
	}
	return nil
}

// ClientGroup represents a subgroup of Lantern clients chosen randomly or
// based on certain criteria on which features can be selectively turned on.
type ClientGroup struct {
	// A label so that the group can be referred to when collecting/analyzing
	// metrics. Better to be unique and meaningful.
	Label string
	// UserFloor and UserCeil defines the range of user IDs so that with
	// precision p, any user ID u satisfies floor*p <= u%p < ceil*p belongs to
	// the group. Precision is expressed in the code and can be changed freely.
	//
	// For example, given floor = 0.1 and ceil = 0.2, it matches user IDs end
	// between 100 and 199 if precision is 1000, and IDs end between 1000 and
	// 1999 if precision is 10000.
	//
	// Range: 0-1. When both are omitted, all users fall within the range.
	UserFloor float64
	UserCeil  float64
	// The application the feature applies to. Defaults to all applications.
	Application string
	// A semantic version range which only Lantern versions falls within is consided.
	// Defaults to all versions.
	VersionConstraints string
	// Comma separated list of platforms the group includes.
	// Defaults to all platforms.
	Platforms string
	// Only include Lantern Free clients.
	FreeOnly bool
	// Only include Lantern Pro clients.
	ProOnly bool
	// Comma separated list of countries the group includes.
	// Defaults to all countries.
	GeoCountries string
	// Random fraction of clients to include from the final set where all other
	// criteria match.
	//
	// Range: 0-1. Defaults to 1.
	Fraction float64
}

// Validate checks if the ClientGroup fields are valid and do not conflict with
// each other.
func (g ClientGroup) Validate() error {
	if g.UserFloor < 0 || g.UserFloor > 1.0 {
		return errors.New("Invalid UserFloor")
	}
	if g.UserCeil < 0 || g.UserCeil > 1.0 {
		return errors.New("Invalid UserCeil")
	}
	if g.UserCeil < g.UserFloor {
		return errors.New("Invalid user range")
	}
	if g.Fraction < 0 || g.Fraction > 1.0 {
		return errors.New("Invalid Fraction")
	}
	if g.FreeOnly && g.ProOnly {
		return errors.New("Both FreeOnly and ProOnly is set")
	}
	if g.VersionConstraints != "" {
		_, err := semver.ParseRange(g.VersionConstraints)
		if err != nil {
			return fmt.Errorf("error parsing version constraints: %v", err)
		}
	}
	return nil
}

// Includes checks if the ClientGroup includes the user, device and country
// combination, assuming the group has been validated.
func (g ClientGroup) Includes(platform, appName, version string, userID int64, isPro bool, geoCountry string) bool {
	if g.UserCeil > 0 {
		// Unknown user ID doesn't belong to any user range
		if userID == 0 {
			return false
		}
		precision := 1000.0
		remainder := userID % int64(precision)
		if remainder < int64(g.UserFloor*precision) || remainder >= int64(g.UserCeil*precision) {
			return false
		}
	}
	if g.FreeOnly && isPro {
		return false
	}
	if g.ProOnly && !isPro {
		return false
	}
	if g.Application != "" && !strings.EqualFold(g.Application, appName) {
		return false
	}
	if g.VersionConstraints != "" {
		expectedRange, err := semver.ParseRange(g.VersionConstraints)
		if err != nil {
			return false
		}
		if !expectedRange(semver.MustParse(version)) {
			return false
		}
	}
	if g.Platforms != "" && !csvContains(g.Platforms, platform) {
		return false
	}
	if g.GeoCountries != "" && !csvContains(g.GeoCountries, geoCountry) {
		return false
	}
	if g.Fraction > 0 && randomFloat >= g.Fraction {
		return false
	}
	return true
}

func csvContains(csv, s string) bool {
	fields := strings.Split(csv, ",")
	for _, f := range fields {
		if strings.EqualFold(s, strings.TrimSpace(f)) {
			return true
		}
	}
	return false
}

func somethingFromMap[T any](m map[string]interface{}, name string) (T, error) {
	var ret T
	v, exists := m[name]
	if !exists {
		return ret, errAbsentOption
	}
	var ok bool
	ret, ok = v.(T)
	if !ok {
		return ret, errMalformedOption
	}
	return ret, nil
}

func durationFromMap(m map[string]interface{}, name string) (time.Duration, error) {
	s, err := somethingFromMap[string](m, name)
	if err != nil {
		return 0, err
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, errMalformedOption
	}
	return d, nil
}
