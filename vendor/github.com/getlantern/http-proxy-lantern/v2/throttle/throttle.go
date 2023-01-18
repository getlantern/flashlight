// Package throttle provides the ability to read throttling configurations from
// redis. Configurations are stored in redis as maps under the keys
// "_throttle:desktop" and "_throttle:mobile". The key/value pairs in each map
// are the 2-digit lowercase ISO-3166 country code plus a pipe-delimited
// threshold and rate, for example:
//
//   _throttle:mobile
//     "__"   "524288000|10240"
//     "cn"   "104857600|10240"
//
package throttle

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/spaolacci/murmur3"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
)

const (
	DefaultRefreshInterval = 5 * time.Minute
)

var (
	log = golog.LoggerFor("flashlight.throttle")
)

type CapInterval string

const (
	Daily   = "daily"
	Weekly  = "weekly"
	Monthly = "monthly"
	Legacy  = "legacy" // like Monthly for old clients
)

type Settings struct {
	// Label uniquely identifies this set of settings for reporting purposes
	Label string

	// AppName constrains this setting to a particular application name. Leave blank to apply to all applications.
	AppName string

	// DeviceFloor is an optional number between 0 and 1 that sets the floor (inclusive) of devices included in the cohort that gets these settings
	DeviceFloor float64

	// DeviceCeil is an optional number between 0 and 1 that sets the floor (exclusive) of devices included in the cohort that gets these settings.
	// If DeviceCeil is 1, the 1 is treated as inclusive.
	DeviceCeil float64

	// Threshold at which we start throttling (in bytes)
	Threshold int64

	// Rate to which to throttle (in bytes per second)
	Rate int64

	// How frequently the usage cap resets, one of "daily", "weekly" or "monthly"
	CapResets CapInterval
}

func (settings *Settings) Validate() error {
	if settings.Label == "" {
		return errors.New("Missing label")
	}

	if settings.CapResets != Daily && settings.CapResets != Weekly && settings.CapResets != Monthly {
		return errors.New("Unknown CapResets interval %v: ", settings.CapResets)
	}

	if settings.Threshold > 0 && settings.Rate <= 0 {
		return errors.New("Throttling threshold specified without a rate")
	}

	return nil
}

// Config is a per-country throttling config
type Config interface {
	// SettingsFor returns the throttling settings for the given deviceID in the given
	// countryCode on the given platform (windows, darwin, linux, android or ios). At the each level
	// (country and platform) this should fall back to default values if a specific value isn't provided.
	// supportedDataCaps identifies which cap intervals the client supports ("daily", "weekly" or "monthly").
	// If this list is empty, the client is assumed to support "monthly" (legacy clients).
	SettingsFor(deviceID, countryCode, platform, appName string, supportedDataCaps []string) (settings *Settings, ok bool)
}

// NewForcedConfig returns a new Config that uses the forced threshold, rate and TTL
func NewForcedConfig(threshold int64, rate int64, capResets CapInterval) Config {
	return &forcedConfig{
		Settings: Settings{
			Label:     "forced",
			Threshold: threshold,
			Rate:      rate,
			CapResets: capResets,
		},
	}
}

type forcedConfig struct {
	Settings
}

func (cfg *forcedConfig) SettingsFor(deviceID, countryCode, platform, appName string, supportedDataCaps []string) (settings *Settings, ok bool) {
	return &cfg.Settings, true
}

// SettingsByCountryAndPlatform organizes slices of SettingsWithConstraints by
// country -> platform
type SettingsByCountryAndPlatform map[string]map[string][]*Settings

func (sbcap SettingsByCountryAndPlatform) Validate() error {
	for _, platforms := range sbcap {
		for _, cohorts := range platforms {
			for _, settings := range cohorts {
				err := settings.Validate()
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func decodeSettingsByCountryAndPlatform(encoded []byte) (settings SettingsByCountryAndPlatform, err error) {
	settings = make(SettingsByCountryAndPlatform)
	err = json.Unmarshal(encoded, &settings)
	return
}

type redisConfig struct {
	rc              *redis.Client
	refreshInterval time.Duration
	settings        SettingsByCountryAndPlatform
	mx              sync.RWMutex
	ctx             context.Context
}

// NewRedisConfig returns a new Config that uses the given redis client to load
// its configuration information and reload that information every
// refreshInterval.
func NewRedisConfig(rc *redis.Client, refreshInterval time.Duration) Config {
	cfg := &redisConfig{
		rc:              rc,
		refreshInterval: refreshInterval,
		ctx:             context.Background(),
	}
	cfg.refreshSettings()
	go cfg.keepCurrent()
	return cfg
}

func (cfg *redisConfig) keepCurrent() {
	if cfg.refreshInterval <= 0 {
		log.Debugf("Defaulting refresh interval to %v", DefaultRefreshInterval)
		cfg.refreshInterval = DefaultRefreshInterval
	}

	log.Debugf("Refreshing every %v", cfg.refreshInterval)
	for {
		time.Sleep(cfg.refreshInterval)
		cfg.refreshSettings()
	}
}

func (cfg *redisConfig) refreshSettings() {
	encoded, err := cfg.rc.Get(cfg.ctx, "_throttle").Bytes()
	if err != nil {
		log.Errorf("Unable to load throttle settings from redis: %v", err)
		return
	}
	settings, err := decodeSettingsByCountryAndPlatform(encoded)
	if err != nil {
		log.Errorf("Unable to decode throttle settings: %v", err)
		return
	}

	log.Debugf("Loaded throttle config: %v", string(encoded))

	cfg.mx.Lock()
	cfg.settings = settings
	cfg.mx.Unlock()
}

func (cfg *redisConfig) SettingsFor(deviceID, countryCode, platform, appName string, supportedDataCaps []string) (*Settings, bool) {
	cfg.mx.RLock()
	settings := cfg.settings
	cfg.mx.RUnlock()

	platformSettings := settings[strings.ToLower(countryCode)]
	if platformSettings == nil {
		log.Tracef("No settings found for country %v, use default", countryCode)
		platformSettings = settings["default"]
		if platformSettings == nil {
			log.Trace("No settings for default country, not throttling")
			return nil, false
		}
	}

	constrainedSettings := platformSettings[strings.ToLower(platform)]
	if len(constrainedSettings) == 0 {
		log.Tracef("No settings found for platform %v, use default", platform)
		constrainedSettings = platformSettings["default"]
		if len(constrainedSettings) == 0 {
			log.Trace("No settings for default platform, not throttling")
			return nil, false
		}
	}

	clientSupportsInterval := func(requested CapInterval) bool {
		if requested == Legacy && len(supportedDataCaps) == 0 {
			// legacy client
			return true
		}
		for _, supported := range supportedDataCaps {
			if requested == CapInterval(supported) {
				return true
			}
		}
		return false
	}

	hash := murmur3.New64()
	hash.Write([]byte(deviceID))
	hashOfDeviceID := hash.Sum64()
	const scale = 1000000 // do not change this, as it will result in users being segmented differently than they were before
	segment := float64((hashOfDeviceID % scale)) / float64(scale)

	settingsForAppName := func(checkAppName string) *Settings {
		for _, candidateSettings := range constrainedSettings {
			if clientSupportsInterval(candidateSettings.CapResets) {
				appMatches := candidateSettings.AppName == checkAppName
				deviceMatches := candidateSettings.DeviceFloor <= segment && (candidateSettings.DeviceCeil > segment || (candidateSettings.DeviceCeil == 1 && segment == 1))
				if appMatches && deviceMatches {
					return candidateSettings
				}
			}
		}

		log.Tracef("No setting for segment %v, using first supported in list", segment)
		for _, candidateSettings := range constrainedSettings {
			if clientSupportsInterval(candidateSettings.CapResets) {
				appMatches := candidateSettings.AppName == checkAppName
				if appMatches {
					return candidateSettings
				}
			}
		}

		return nil
	}

	result := settingsForAppName(appName)
	if result == nil && appName != "" {
		log.Tracef("No applicable settings found for app name %v, trying with no app name", appName)
		result = settingsForAppName("")
	}

	return result, result != nil
}
