package client

import (
	"github.com/getlantern/fronted"
)

const (
	// any masquerade that does not have a provider is assigned this provider
	// eg if encountered in a cache file etc.
	defaultZeroProviderID = "cloudfront"
)

// ClientConfig captures configuration information for a Client
type ClientConfig struct {
	DumpHeaders bool // whether or not to dump headers of requests and responses

	Fronted *FrontedConfig

	// Legacy masquerade configuration (cloudfront only).
	// older configuration files will contain cloudfront
	// masquerade configuration here.  Possible to
	// encounter this on disk.
	MasqueradeSets map[string][]*fronted.Masquerade
}

type FrontedConfig struct {
	ZeroProviderID string // provider id used when none is known (legacy config)
	Providers      map[string]*fronted.Provider
}

// NewConfig creates a new client config with default values.
func NewConfig() *ClientConfig {
	f := &FrontedConfig{
		ZeroProviderID: defaultZeroProviderID,
		Providers:      make(map[string]*fronted.Provider),
	}

	return &ClientConfig{
		Fronted:        f,
		MasqueradeSets: make(map[string][]*fronted.Masquerade),
	}
}

// applyDefaults creates any default values that cannot be zero-value initialized
func (c *ClientConfig) ApplyDefaults() {
	c.checkZeroProvider()
}

// If an old configuration is loaded which does not specify
// any fronted provider details (only masqeradesets), add a
// provider with the old hardcoded details (cloudfront) and
// the list of masquerades given in masqueradesets.
//
// Note: if any other provider is specified or a cloudfront
// configuration is present, this provider will not be added.
// It is assumed in that case if cloudfront is enabled, it will
// be explicitly listed.
func (c *ClientConfig) checkZeroProvider() {
	if len(c.Fronted.Providers) == 0 {
		providerID := c.Fronted.ZeroProviderID
		cfHosts := map[string]string{
			"api.getiantem.org":                "d2n32kma9hyo9f.cloudfront.net",
			"api-staging.getiantem.org":        "d16igwq64x5e11.cloudfront.net",
			"borda.lantern.io":                 "d157vud77ygy87.cloudfront.net",
			"config.getiantem.org":             "d2wi0vwulmtn99.cloudfront.net",
			"config-staging.getiantem.org":     "d33pfmbpauhmvd.cloudfront.net",
			"geo.getiantem.org":                "d3u5fqukq7qrhd.cloudfront.net",
			"globalconfig.flashlightproxy.com": "d24ykmup0867cj.cloudfront.net",
			"update.getlantern.org":            "d2yl1zps97e5mx.cloudfront.net",
		}
		cfTestURL := "http://d157vud77ygy87.cloudfront.net/ping"

		c.Fronted.Providers[providerID] = fronted.NewProvider(cfHosts, cfTestURL, c.MasqueradeSets[providerID])
		log.Debugf("Added default provider details for '%s'", providerID)
	}
}
