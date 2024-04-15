package client

import (
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/config"
	"github.com/getlantern/flashlight/v7/geolookup"
)

var (
	// blockingRelevantFeatures lists all features that might affect blocking and gives their
	// default enabled status (until we know the country)
	blockingRelevantFeatures = map[string]bool{
		config.FeatureProxyBench:           false,
		config.FeatureGoogleSearchAds:      false,
		config.FeatureNoBorda:              true,
		config.FeatureProbeProxies:         false,
		config.FeatureDetour:               false,
		config.FeatureNoHTTPSEverywhere:    true,
		config.FeatureProxyWhitelistedOnly: true,
	}
)

// EnabledFeatures gets all features enabled based on current conditions
func (client *Client) EnabledFeatures() map[string]bool {
	featuresEnabled := make(map[string]bool)
	client.mxGlobal.RLock()
	if client.global == nil {
		client.mxGlobal.RUnlock()
		return featuresEnabled
	}
	global := client.global
	client.mxGlobal.RUnlock()
	country := geolookup.GetCountry(0)
	for feature := range global.FeaturesEnabled {
		if client.calcFeature(global, country, "0.0.1", feature) {
			featuresEnabled[feature] = true
		}
	}
	return featuresEnabled
}

func (client *Client) allowHTTPSEverywhere() bool {
	return !client.featureEnabled(config.FeatureNoHTTPSEverywhere)
}

func (client *Client) proxyAll() bool {
	useShortcutOrDetour := client.useShortcut() || client.useDetour()
	return !useShortcutOrDetour && !client.featureEnabled(config.FeatureProxyWhitelistedOnly)
}

func (client *Client) useDetour() bool {
	if client.callbacks.useDetour != nil {
		return client.callbacks.useDetour()
	}
	return client.featureEnabled(config.FeatureDetour) && !client.featureEnabled(config.FeatureProxyWhitelistedOnly)
}

func (client *Client) useShortcut() bool {
	if client.callbacks.useShortcut != nil {
		return client.callbacks.useShortcut()
	}
	return client.featureEnabled(config.FeatureShortcut) && !client.featureEnabled(config.FeatureProxyWhitelistedOnly)
}

// featureEnabled returns true if the input feature is enabled for this flashlight instance. Feature
// names are tracked in the config package.
func (client *Client) featureEnabled(feature string) bool {
	// features internal to flashlight are not controllable by application version, since flashlight doesn't know the version, so we use a very low version number just to make sure it parses
	return client.FeatureEnabled(feature, "0.0.1")
}

func (client *Client) FeatureEnabled(feature, applicationVersion string) bool {
	client.mxGlobal.RLock()
	global := client.global
	client.mxGlobal.RUnlock()
	return client.calcFeature(global, geolookup.GetCountry(0), applicationVersion, feature)
}

func (client *Client) calcFeature(global *config.Global, country, applicationVersion, feature string) bool {
	// Special case: Use defaults for blocking related features until geolookup is finished
	// to avoid accidentally generating traffic that could trigger blocking.
	enabled, blockingRelated := blockingRelevantFeatures[feature]
	if country == "" && blockingRelated {
		enabledText := "disabled"
		if enabled {
			enabledText = "enabled"
		}
		log.Debugf("Blocking related feature %v %v because geolookup has not yet finished", feature, enabledText)
		return enabled
	}
	if global == nil {
		log.Error("No global configuration!")
		return enabled
	}
	if blockingRelated {
		log.Debugf("Checking blocking related feature %v with country set to %v", feature, country)
	}
	return global.FeatureEnabled(feature,
		common.Platform,
		client.user.GetAppName(),
		applicationVersion,
		client.user.GetUserID(),
		client.isPro(),
		country)
}

// FeatureOptions unmarshals options for the input feature. Feature names are tracked in the config
// package.
func (client *Client) FeatureOptions(feature string, opts config.FeatureOptions) error {
	client.mxGlobal.RLock()
	global := client.global
	client.mxGlobal.RUnlock()
	if global == nil {
		// just to be safe
		return errors.New("No global configuration")
	}
	return global.UnmarshalFeatureOptions(feature, opts)
}
