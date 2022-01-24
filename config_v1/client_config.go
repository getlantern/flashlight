package config

import (
	"errors"

	"github.com/getlantern/fronted"
)

const (
	// for historical reasons, if a provider is unspecified in a masquerade, it
	// is treated as a cloudfront masquerade (which was once the only provider)
	DefaultFrontedProviderID = "cloudfront"
)

// ClientConfig captures configuration information for a Client
type ClientConfig struct {
	DumpHeaders bool // whether or not to dump headers of requests and responses
	Fronted     *FrontedConfig

	// Legacy masquerade configuration
	// included to test presence for older clients
	MasqueradeSets map[string][]*fronted.Masquerade
}

// Configuration structure for direct domain fronting
type FrontedConfig struct {
	Providers map[string]*ProviderConfig
}

// Configuration structure for a particular fronting provider (cloudfront, akamai, etc)
type ProviderConfig struct {
	HostAliases         map[string]string
	TestURL             string
	Masquerades         []*fronted.Masquerade
	Validator           *ValidatorConfig
	PassthroughPatterns []string
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
		Fronted: newFrontedConfig(),
	}
}

// Builds a list of fronted.Providers to use based on the configuration
func (c *ClientConfig) FrontedProviders() map[string]*fronted.Provider {

	providers := make(map[string]*fronted.Provider)
	for pid, p := range c.Fronted.Providers {
		providers[pid] = fronted.NewProvider(
			p.HostAliases,
			p.TestURL,
			p.Masquerades,
			p.GetResponseValidator(pid),
			p.PassthroughPatterns,
		)
	}
	return providers
}

// Check that this ClientConfig is valid
func (c *ClientConfig) Validate() error {
	sz := 0
	for _, p := range c.Fronted.Providers {
		sz += len(p.Masquerades)
	}
	if sz == 0 {
		return errors.New("No masquerades.")
	}

	return nil
}
