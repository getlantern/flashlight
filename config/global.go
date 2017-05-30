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
	Client                *client.ClientConfig

	// ProxiedSites are domains that get routed through Lantern rather than accessed directly.
	ProxiedSites *proxiedsites.Config

	// TrustedCAs are trusted CAs for domain fronting domains only.
	TrustedCAs []*fronted.CA
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

func (cfg *Global) validate() error {
	if len(cfg.Client.MasqueradeSets) == 0 {
		return errors.New("No masquerades")
	}
	if len(cfg.TrustedCAs) == 0 {
		return errors.New("No trusted CAs")
	}
	return nil
}
