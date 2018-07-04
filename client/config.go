package client

import (
	"github.com/getlantern/fronted"
)

const (
	// Cloudfront defaults used in prior versions

	// any masquerade that does not have a provider is assigned this provider
	// eg if encountered in a cache file etc.
	CloudfrontProviderID = "cloudfront"
	CloudfrontTestURL    = "http://d157vud77ygy87.cloudfront.net/ping"
)

var (
	cloudfrontHostAliases = map[string]string{
		"api.getiantem.org":                "d2n32kma9hyo9f.cloudfront.net",
		"api-staging.getiantem.org":        "d16igwq64x5e11.cloudfront.net",
		"borda.lantern.io":                 "d157vud77ygy87.cloudfront.net",
		"config.getiantem.org":             "d2wi0vwulmtn99.cloudfront.net",
		"config-staging.getiantem.org":     "d33pfmbpauhmvd.cloudfront.net",
		"geo.getiantem.org":                "d3u5fqukq7qrhd.cloudfront.net",
		"globalconfig.flashlightproxy.com": "d24ykmup0867cj.cloudfront.net",
		"update.getlantern.org":            "d2yl1zps97e5mx.cloudfront.net",
		"github.com":                       "d2yl1zps97e5mx.cloudfront.net",
	}
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
	Providers map[string]*fronted.Provider
}

func newFrontedConfig() *FrontedConfig {
	return &FrontedConfig{
		Providers: make(map[string]*fronted.Provider),
	}
}

// NewConfig creates a new client config with default values.
func NewConfig() *ClientConfig {
	return &ClientConfig{
		Fronted:        newFrontedConfig(),
		MasqueradeSets: make(map[string][]*fronted.Masquerade),
	}
}

// applyDefaults creates any default values that cannot be zero-value initialized
func (c *ClientConfig) ApplyDefaults() {
	c.checkNoFrontedConfig()
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
func (c *ClientConfig) checkNoFrontedConfig() {
	// yaml can nil out these fields in some cases
	if c.Fronted == nil {
		c.Fronted = newFrontedConfig()
	} else if c.Fronted.Providers == nil {
		c.Fronted.Providers = make(map[string]*fronted.Provider)
	}
	if len(c.Fronted.Providers) == 0 {
		pid := CloudfrontProviderID
		c.Fronted.Providers[pid] = fronted.NewProvider(cloudfrontHostAliases, CloudfrontTestURL, c.MasqueradeSets[pid])
		log.Debugf("Added default provider details for '%s'", pid)
	} else {
		log.Debugf("Using configured providers.")
	}
}

func GetCloudfrontHostAliases() map[string]string {
	a := make(map[string]string)
	for k, v := range cloudfrontHostAliases {
		a[k] = v
	}
	return a
}
