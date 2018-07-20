package config

import (
	"errors"
	"time"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/fronted"
	"github.com/getlantern/proxiedsites"
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
	ReportIssueEmail      string
	Client                *client.ClientConfig

	// AdSettings are the settings to use for showing ads to mobile clients
	AdSettings *AdSettings

	// ProxiedSites are domains that get routed through Lantern rather than accessed directly.
	ProxiedSites *proxiedsites.Config

	// TrustedCAs are trusted CAs for domain fronting domains only.
	TrustedCAs []*fronted.CA

	// GlobalConfigPollInterval sets interval at which to poll for global config
	GlobalConfigPollInterval time.Duration

	// ProxyConfigPollInterval sets interval at which to poll for proxy config
	ProxyConfigPollInterval time.Duration
}

// newGlobal creates a new global config with otherwise nil values set.
func newGlobal() *Global {
	return &Global{
		Client: client.NewConfig(),
		ProxiedSites: &proxiedsites.Config{
			Delta: &proxiedsites.Delta{},
		},
	}
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
	return nil
}
