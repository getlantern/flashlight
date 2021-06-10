package config

import (
	"crypto/x509"
	"errors"
	"time"

	"github.com/getlantern/flashlight/browsers/simbrowser"
	"github.com/getlantern/flashlight/domainrouting"
	"github.com/getlantern/fronted"
	"github.com/getlantern/keyman"
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
	DomainRoutingRules domainrouting.Rules

	// TrustedCAs are trusted CAs for domain fronting domains only.
	TrustedCAs []*fronted.CA

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

	Replica *ReplicaConfig
}

// NewGlobal creates a new global config with otherwise nil values set.
func NewGlobal() *Global {
	return &Global{
		Client:       NewClientConfig(),
		ProxiedSites: &domainrouting.ProxiedSitesConfig{},
	}
}

// FeatureEnabled checks if the feature is enabled given the client properties.
func (cfg *Global) FeatureEnabled(feature string, userID int64, isPro bool,
	geoCountry string) bool {
	enabled, _ := cfg.FeatureEnabledWithLabel(feature, userID, isPro, geoCountry)
	log.Tracef("Feature %v enabled for user %v in country %v?: %v", feature, userID, geoCountry, enabled)
	return enabled
}

// FeatureEnabledWithLabel is the same as FeatureEnabled but also returns the
// label of the first matched ClientGroup if the feature is enabled.
func (cfg *Global) FeatureEnabledWithLabel(feature string, userID int64, isPro bool,
	geoCountry string) (enabled bool, label string) {
	groups, exists := cfg.FeaturesEnabled[feature]
	if !exists {
		return false, ""
	}
	for _, g := range groups {
		if g.Includes(userID, isPro, geoCountry) {
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
	return opts.fromMap(m)
}

// TrustedCACerts returns a certificate pool containing the TrustedCAs from this
// config.
func (cfg *Global) TrustedCACerts() (pool *x509.CertPool, err error) {
	certs := make([]string, 0, len(cfg.TrustedCAs))
	for _, ca := range cfg.TrustedCAs {
		certs = append(certs, ca.Cert)
	}
	pool, err = keyman.PoolContainingCerts(certs...)
	if err != nil {
		log.Errorf("Could not create pool %v", err)
	}
	return
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
	err := cfg.Client.Validate()
	if err != nil {
		return err
	}
	if len(cfg.TrustedCAs) == 0 {
		return errors.New("No trusted CAs")
	}
	for _, groups := range cfg.FeaturesEnabled {
		for _, g := range groups {
			if err := g.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}
