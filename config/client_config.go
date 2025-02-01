package config

import (
	"github.com/getlantern/fronted"
)

// ClientConfig captures configuration information for a Client
type ClientConfig struct {
	DumpHeaders bool // whether or not to dump headers of requests and responses
	Fronted     *FrontedConfig

	// Legacy masquerade configuration
	// included to test presence for older clients
	MasqueradeSets map[string][]*fronted.Masquerade

	// DNS host-to-ip mappings to bypass DNS when resolving hostnames
	// This only works for direct dials (i.e., domain routing rules that are
	// MustDirect or 'md':
	// https://github.com/getlantern/flashlight/blob/f82d9ab04da841e4a2833783b244948a8daa547e/domainrouting/domainrouting.go#L33
	DNSResolutionMapForDirectDials map[string]string
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
	FrontingSNIs        map[string]*fronted.SNIConfig
	VerifyHostname      *string `yaml:"verifyHostname,omitempty"`
}

// returns a fronted.ResponseValidator specified by the
// provider config or nil if none was specified
func (p *ProviderConfig) GetResponseValidator(providerID string) fronted.ResponseValidator {
	// hard-coded custom validators can be determined here if needed...

	if p.Validator == nil {
		return nil
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
