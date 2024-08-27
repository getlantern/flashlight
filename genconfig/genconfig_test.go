package main

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"testing"

	"github.com/getlantern/flashlight/v7/config"
	"github.com/getlantern/flashlight/v7/domainrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// This variable is only being used for test purposes!
// The original global.yaml.tmpl is located at flashlight repository: embbededconfig/global.yaml.tmpl
//
//go:embed testdata/global_test.yaml.tmpl
var globalTemplateTest string

//go:embed testdata/blacklist.txt
var blacklistTest []byte

func TestGenerateConfig(t *testing.T) {
	require.NotEmpty(t, globalTemplateTest)

	ctx := context.Background()

	masquerades = []string{"96.16.55.171 a248.e.akamai.net akamai", "104.123.154.46 a248.e.akamai.net akamai", "3.164.129.16 Smentertainment.com cloudfront", "204.246.164.205 Smentertainment.com cloudfront"}
	proxiedSites := filter{"googlevideo.com": true, "googleapis.com": true, "google.com": true}
	blacklist := make(filter)

	loadFilterList(blacklistTest, &blacklist)
	//loadProxiedSitesList()
	//loadBlacklist()

	var tests = []struct {
		name                 string
		givenContext         context.Context
		givenTemplate        string
		givenMasquerades     []string
		givenProxiedSites    filter
		givenBlacklist       filter
		givenNumberOfWorkers int
		givenMinFrequency    float64
		givenMinMasquerades  int
		givenMaxMasquerades  int
		assert               func(*testing.T, string, error)
		setup                func() *ConfigGenerator
	}{
		{
			name:                 "should generate config with success",
			givenContext:         ctx,
			givenTemplate:        globalTemplateTest,
			givenMasquerades:     masquerades,
			givenProxiedSites:    proxiedSites,
			givenBlacklist:       blacklist,
			givenNumberOfWorkers: 10,
			givenMinFrequency:    10,
			givenMinMasquerades:  1,
			givenMaxMasquerades:  10,
			assert: func(t *testing.T, cfg string, err error) {
				require.NoError(t, err)
				require.NotEmpty(t, cfg)
				globalConfig, err := parseGlobal(ctx, []byte(cfg))
				require.NoError(t, err)
				assert.NotNil(t, globalConfig)
				assert.NotNil(t, globalConfig.Client)
				assert.NotNil(t, globalConfig.Client.Fronted)
				assert.NotNil(t, globalConfig.Client.Fronted.Providers)
				assert.Contains(t, globalConfig.Client.Fronted.Providers, "cloudfront")
				assert.Contains(t, globalConfig.Client.Fronted.Providers, "akamai")
				assert.Contains(t, globalConfig.Client.MasqueradeSets, "cloudfront")
			},
			setup: func() *ConfigGenerator {
				generator := NewConfigGenerator()
				for _, provider := range generator.Providers {
					provider.Enabled = true
				}
				require.NotNil(t, generator.Providers[defaultProviderID])
				return generator
			},
		},
		{
			name: "should generate config with success even with cloudfront disabled",
			setup: func() *ConfigGenerator {
				configGenerator := NewConfigGenerator()
				configGenerator.Providers["cloudfront"].Enabled = false
				configGenerator.Providers["akamai"].Enabled = true
				return configGenerator
			},
			givenContext:         ctx,
			givenTemplate:        globalTemplateTest,
			givenMasquerades:     masquerades,
			givenProxiedSites:    proxiedSites,
			givenBlacklist:       blacklist,
			givenNumberOfWorkers: 10,
			givenMinFrequency:    10,
			givenMinMasquerades:  1,
			givenMaxMasquerades:  10,
			assert: func(t *testing.T, cfg string, err error) {
				require.NoError(t, err)
				require.NotEmpty(t, cfg)
				require.NoError(t, err)

				globalConfig, err := parseGlobal(ctx, []byte(cfg))
				require.NoError(t, err)
				assert.NotNil(t, globalConfig)
				assert.NotNil(t, globalConfig.Client)
				assert.NotNil(t, globalConfig.Client.Fronted)
				assert.NotNil(t, globalConfig.Client.Fronted.Providers)
				assert.Contains(t, globalConfig.Client.MasqueradeSets, "cloudfront")
				assert.Len(t, globalConfig.Client.MasqueradeSets["cloudfront"], 2)
				assert.NotContains(t, globalConfig.Client.Fronted.Providers, "cloudfront")
				assert.Contains(t, globalConfig.Client.Fronted.Providers, "akamai")
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			generator := tt.setup()
			cfg, err := generator.GenerateConfig(
				tt.givenContext,
				tt.givenTemplate,
				tt.givenMasquerades,
				tt.givenProxiedSites,
				tt.givenBlacklist,
				tt.givenNumberOfWorkers,
				tt.givenMinFrequency,
				tt.givenMinMasquerades,
				tt.givenMaxMasquerades)
			tt.assert(t, string(cfg), err)
		})
	}
}

func parseGlobal(ctx context.Context, bytes []byte) (*config.Global, error) {
	cfg := &config.Global{}
	err := yaml.Unmarshal(bytes, cfg)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse global config: %v", err)
	}

	var direct, proxied int
	for _, rule := range cfg.DomainRoutingRules {
		switch rule {
		case domainrouting.Direct:
			direct++
		case domainrouting.Proxy:
			proxied++
		}
	}

	return cfg, nil
}

func loadFilterList(data []byte, res *filter) {
	for _, domain := range strings.Split(string(data), "\n") {
		if domain != "" {
			(*res)[domain] = true
		}
	}
}
