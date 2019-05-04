package config

import (
	"errors"

	"github.com/getlantern/fronted"
)

const (
	// Cloudfront defaults used in prior versions

	// any masquerade that does not have a provider is assigned this provider
	// eg if encountered in a cache file etc.
	CloudfrontProviderID = "cloudfront"
	// this url is used to 'vet' cloudfront masquerades
	CloudfrontTestURL = "http://d157vud77ygy87.cloudfront.net/ping"
)

var (
	cloudfrontBadStatus   = []int{403}
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
		"github-production-release-asset-2e65be.s3.amazonaws.com": "d37kom4pw4aa7b.cloudfront.net",
		"mandrillapp.com": "d2rh3u0miqci5a.cloudfront.net",
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

// Configuration structure for direct domain fronting
type FrontedConfig struct {
	Providers map[string]*ProviderConfig
}

// Configuration structure for a parciular fronting provider (cloudfront, akamai, etc)
type ProviderConfig struct {
	HostAliases map[string]string
	TestURL     string
	Masquerades []*fronted.Masquerade
	Validator   *ValidatorConfig
}

// returns a fronted.ResponseValidator specified by the
// provider config or nil if none was specified
func (p *ProviderConfig) GetResponseValidator(providerID string) fronted.ResponseValidator {
	// hard-coded custom validators can be determined here if needed...

	if p.Validator == nil {
		return nil
	}

	if len(p.Validator.RejectStatus) > 0 {
		return fronted.NewStatusCodeValidator(p.Validator.RejectStatus)
	}
	// ...

	// unknown or empty
	return nil
}

// Configuration struture that specifies a fronted.ResponseValidator
type ValidatorConfig struct {
	RejectStatus []int
}

func newFrontedConfig() *FrontedConfig {
	return &FrontedConfig{
		Providers: make(map[string]*ProviderConfig),
	}
}

// NewClientConfig creates a new client config with default values.
func NewClientConfig() *ClientConfig {
	return &ClientConfig{
		Fronted:        newFrontedConfig(),
		MasqueradeSets: make(map[string][]*fronted.Masquerade),
	}
}

// Builds a list of fronted.Providers to use based on the configuration
func (c *ClientConfig) FrontedProviders() map[string]*fronted.Provider {

	providers := make(map[string]*fronted.Provider)

	// If an old configuration is loaded which does not specify
	// any fronted provider details (only masqeradesets)
	// materialize a provider with the old hardcoded cloudfront details
	// using the list of masquerades given in masqueradesets.
	//
	// Note: if any other provider is specified or a cloudfront
	// configuration is present, this provider will not be added.
	// It is assumed in that case if cloudfront is enabled, it will
	// be explicitly listed.
	if c.Fronted == nil || len(c.Fronted.Providers) == 0 {
		pid := CloudfrontProviderID
		providers[pid] = fronted.NewProvider(
			cloudfrontHostAliases,
			CloudfrontTestURL,
			c.MasqueradeSets[pid],
			fronted.NewStatusCodeValidator(cloudfrontBadStatus),
		)
		log.Debugf("Added default provider details for '%s'", pid)
	} else {
		for pid, p := range c.Fronted.Providers {
			providers[pid] = fronted.NewProvider(
				p.HostAliases,
				p.TestURL,
				p.Masquerades,
				p.GetResponseValidator(pid),
			)
		}
	}

	return providers
}

// Check that this ClientConfig is valid
func (c *ClientConfig) Validate() error {
	sz := 0
	if c.Fronted == nil || len(c.Fronted.Providers) == 0 {
		for _, m := range c.MasqueradeSets {
			sz += len(m)
		}
	} else {
		for _, p := range c.Fronted.Providers {
			sz += len(p.Masquerades)
		}
	}

	if sz == 0 {
		return errors.New("No masquerades.")
	}

	return nil
}

// Returns list of http status codes considered to indicate that
// domain fronting failed.
func CloudfrontBadStatus() []int {
	b := make([]int, len(cloudfrontBadStatus))
	copy(b, cloudfrontBadStatus)
	return b
}

// Returns list of host aliases for the default cloudfront
// provider.
func CloudfrontHostAliases() map[string]string {
	a := make(map[string]string)
	for k, v := range cloudfrontHostAliases {
		a[k] = v
	}
	return a
}
