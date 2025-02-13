package config

import (
	"time"

	"github.com/getlantern/flashlight/v7/browsers/simbrowser"
	"github.com/getlantern/flashlight/v7/domainrouting"
	"github.com/getlantern/flashlight/v7/embeddedconfig"
	"github.com/getlantern/flashlight/v7/otel"
)

// Global contains general configuration for Lantern either set globally via
// the cloud, in command line flags, or in local customizations during
// development.
type Global struct {
	Version       int
	CloudConfigCA string
	// AutoUpdateCA is the CA key to pin for auto-updates.
	AutoUpdateCA          string
	UpdateServerURL       string
	BordaReportInterval   time.Duration
	BordaSamplePercentage float64
	// ReportIssueEmail is the recipient of the email sent when the user
	// reports issue.
	ReportIssueEmail string

	// AdSettings are the settings to use for showing ads to mobile clients
	AdSettings *AdSettings

	Client *ClientConfig

	// ProxiedSites are domains that get routed through Lantern rather than accessed directly.
	// This has been deprecated in favor of more precise DomainRoutingRules (see below).
	// The client will continue to honor ProxiedSites configuration for now.
	ProxiedSites *domainrouting.ProxiedSitesConfig

	// DomainRoutingRules specifies routing rules for specific domains, such as forcing proxing, forcing direct dials, etc.
	DomainRoutingRules domainrouting.RulesMap

	// NamedDomainRoutingRules specifies routing rules for specific domains, grouped by name.
	NamedDomainRoutingRules map[string]domainrouting.RulesMap

	// TrustedCAs are trusted CAs for domain fronting domains only.
	//TrustedCAs []*fronted.CA

	// GlobalConfigPollInterval sets interval at which to poll for global config
	GlobalConfigPollInterval time.Duration

	// ProxyConfigPollInterval sets interval at which to poll for proxy config
	ProxyConfigPollInterval time.Duration

	// FeaturesEnabled specifies which optional feature is enabled for certain
	// groups of clients.
	FeaturesEnabled map[string][]*ClientGroup
	// FeatureOptions is a generic way to specify options for optional
	// features. It's up to the feature code to handle the raw JSON message.
	FeatureOptions map[string]map[string]interface{}

	// Market share data used by the simbrowser package when picking a browser to simulate.
	GlobalBrowserMarketShareData   simbrowser.MarketShareData
	RegionalBrowserMarketShareData map[simbrowser.CountryCode]simbrowser.MarketShareData

	// Configuration for OpenTelemetry
	Otel *otel.Config
}

// NewGlobal creates a new global config with otherwise nil values set.
func NewGlobal() *Global {
	return &Global{
		Client:       NewClientConfig(),
		ProxiedSites: &domainrouting.ProxiedSitesConfig{},
	}
}

// FeatureEnabled checks if the feature is enabled given the client properties.
func (cfg *Global) FeatureEnabled(feature, platform, appName, version string, userID int64, isPro bool,
	geoCountry string) bool {
	enabled, _ := cfg.FeatureEnabledWithLabel(feature, platform, appName, version, userID, isPro, geoCountry)
	log.Tracef("Feature %v enabled for user %v in country %v?: %v", feature, userID, geoCountry, enabled)
	return enabled
}

// FeatureEnabledWithLabel is the same as FeatureEnabled but also returns the
// label of the first matched ClientGroup if the feature is enabled.
func (cfg *Global) FeatureEnabledWithLabel(feature, platform, appName, version string, userID int64, isPro bool,
	geoCountry string) (enabled bool, label string) {
	groups, exists := cfg.FeaturesEnabled[feature]
	if !exists {
		return false, ""
	}
	for _, g := range groups {
		if g.Includes(platform, appName, version, userID, isPro, geoCountry) {
			return true, g.Label
		}
	}
	return false, ""
}

func (cfg *Global) UnmarshalFeatureOptions(feature string, opts FeatureOptions) error {
	m, exists := cfg.FeatureOptions[feature]
	if !exists {
		return errAbsentOption
	}
	return opts.FromMap(m)
}

// applyFlags updates this config from any command-line flags that were passed
// in.
func (cfg *Global) applyFlags(flags map[string]interface{}) {
	// Visit all flags that have been set and copy to config
	for key, value := range flags {
		switch key {
		case "cloudconfigca":
			cfg.CloudConfigCA = value.(string)
		case "borda-report-interval":
			cfg.BordaReportInterval = value.(time.Duration)
		case "borda-sample-percentage":
			cfg.BordaSamplePercentage = value.(float64)
		}
	}
}

func (cfg *Global) validate() error {
	for _, groups := range cfg.FeaturesEnabled {
		for _, g := range groups {
			if err := g.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

// Returns the global config in structured form, by executing the template without any data. This is useful for consuming parts of the config that aren't templatized.
func GetEmbeddedGlobalSansTemplateData() (*Global, error) {
	var g Global
	err := embeddedconfig.ExecuteAndUnmarshalGlobal(nil, &g)
	return &g, err
}
