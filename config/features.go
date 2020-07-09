package config

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/blang/semver"

	"github.com/getlantern/flashlight/common"
)

const (
	FeatureProxyBench           = "proxybench"
	FeaturePingProxies          = "pingproxies"
	FeatureTrafficLog           = "trafficlog"
	FeatureNoBorda              = "noborda"
	FeatureNoProbeProxies       = "noprobeproxies"
	FeatureNoShortcut           = "noshortcut"
	FeatureNoDetour             = "nodetour"
	FeatureNoHTTPSEverywhere    = "nohttpseverywhere"
	FeatureProxyWhitelistedOnly = "proxywhitelistedonly"
	FeatureTrackYouTube         = "trackyoutube"
)

var (
	// to have stable calculation of fraction until the client restarts.
	randomFloat = rand.Float64()

	errAbsentOption    = errors.New("option is absent")
	errMalformedOption = errors.New("malformed option")
)

// FeatureOptions is an interface implemented by all feature options
type FeatureOptions interface {
	fromMap(map[string]interface{}) error
}

type PingProxiesOptions struct {
	Interval time.Duration
}

func (o *PingProxiesOptions) fromMap(m map[string]interface{}) error {
	interval, err := durationFromMap(m, "interval")
	if err != nil {
		return err
	}
	o.Interval = interval
	return nil
}

type TrafficLogOptions struct {
	// Size of the buffers for capturing packets in real time.
	CaptureBytes int
	// Size of the buffers for taking snapshots from live captures.
	SaveBytes int
}

func (o *TrafficLogOptions) fromMap(m map[string]interface{}) error {
	captureBytes, err := intFromMap(m, "capturebytes")
	if err != nil {
		return err
	}
	o.CaptureBytes = captureBytes
	saveBytes, err := intFromMap(m, "savebytes")
	if err != nil {
		return err
	}
	o.SaveBytes = saveBytes
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
			return fmt.Errorf("Error parsing version constraints: %v", err)
		}
	}
	return nil
}

//Includes checks if the ClientGroup includes the user, device and country
//combination, assuming the group has been validated.
func (g ClientGroup) Includes(userID int64, isPro bool, geoCountry string) bool {
	if g.UserCeil > 0 {
		percision := 1000.0
		remainder := userID % int64(percision)
		if remainder < int64(g.UserFloor*percision) || remainder >= int64(g.UserCeil*percision) {
			return false
		}
	}
	if g.FreeOnly && isPro {
		return false
	}
	if g.ProOnly && !isPro {
		return false
	}
	if g.VersionConstraints != "" {
		expectedRange, err := semver.ParseRange(g.VersionConstraints)
		if err != nil {
			return false
		}
		if !expectedRange(semver.MustParse(common.Version)) {
			return false
		}
	}
	if g.Platforms != "" && !csvContains(g.Platforms, common.Platform) {
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

func intFromMap(m map[string]interface{}, name string) (int, error) {
	v, exists := m[name]
	if !exists {
		return 0, errAbsentOption
	}
	i, ok := v.(int)
	if !ok {
		return 0, errMalformedOption
	}
	return i, nil
}

func durationFromMap(m map[string]interface{}, name string) (time.Duration, error) {
	v, exists := m[name]
	if !exists {
		return 0, errAbsentOption
	}
	s, ok := v.(string)
	if !ok {
		return 0, errMalformedOption
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, errMalformedOption
	}
	return d, nil
}
