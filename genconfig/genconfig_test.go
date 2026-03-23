package main

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/getlantern/keyman"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v2"

	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/config"
	"github.com/getlantern/flashlight/v7/domainrouting"
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

	// create a tls listener
	pk, err := keyman.GeneratePK(2048)
	require.NoError(t, err)

	// Generate self-signed certificate
	cert, err := pk.TLSCertificateFor(time.Now().Add(1*time.Hour), true, nil, "tlsdialer", "localhost", "127.0.0.1")
	require.NoError(t, err)

	masquerades = []string{"127.0.0.1:9999 127.0.0.1 akamai", "127.0.0.1:9999 127.0.0.1 akamai", "127.0.0.1:9999 127.0.0.1 cloudfront", "127.0.0.1:9999 127.0.0.1 cloudfront"}
	proxiedSites := filter{"googlevideo.com": true, "googleapis.com": true, "google.com": true}
	blacklist := make(filter)

	loadFilterList(blacklistTest, &blacklist)

	dnsttCfg := &common.DNSTTConfig{
		Domain:           "t.iantem.io",
		PublicKey:        "abcd1234",
		DoHResolver:      "https://doh.example.com/dns-query",
		DoTResolver:      "",
		UTLSDistribution: "chrome",
	}

	ampCacheConfig := &common.AMPCacheConfig{
		BrokerURL:    "https://amp.broker.com",
		CacheURL:     "https://amp.cache.com",
		PublicKeyPEM: "pem",
		FrontDomains: []string{"pudim.com.br"},
	}

	defaultSetup := func(ctrl *gomock.Controller) *ConfigGenerator {
		configGenerator := NewConfigGenerator()

		verifier := NewMockverifier(ctrl)
		verifier.EXPECT().Vet(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		configGenerator.verifier = verifier

		certGrabber := NewMockcertGrabber(ctrl)
		certGrabber.EXPECT().GetCertificate(gomock.Any(), gomock.Any()).Return(cert, nil).AnyTimes()
		configGenerator.certGrabber = certGrabber

		return configGenerator
	}

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
		givenDNSTTConfig     *common.DNSTTConfig
		givenAMPCacheConfig  *common.AMPCacheConfig
		assert               func(*testing.T, string, error)
		setup                func(*gomock.Controller) *ConfigGenerator
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
			givenDNSTTConfig:     dnsttCfg,
			givenAMPCacheConfig:  ampCacheConfig,
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
				assert.NotNil(t, globalConfig.Client.Fronted.Providers["akamai"].VerifyHostname)
				assert.Equal(t, *globalConfig.Client.Fronted.Providers["akamai"].VerifyHostname, "akamai.com")

				assert.NotNil(t, globalConfig.DNSTTConfig, "DNSTT config should not be nil")
				assert.Equal(t, dnsttCfg, globalConfig.DNSTTConfig, "DNSTT config should match the provided configuration")
			},
			setup: func(ctrl *gomock.Controller) *ConfigGenerator {
				generator := NewConfigGenerator()
				for _, provider := range generator.Providers {
					provider.Enabled = true
				}
				verifier := NewMockverifier(ctrl)
				verifier.EXPECT().Vet(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
				certGrabber := NewMockcertGrabber(ctrl)
				certGrabber.EXPECT().GetCertificate(gomock.Any(), gomock.Any()).Return(cert, nil).AnyTimes()
				generator.certGrabber = certGrabber
				generator.verifier = verifier
				require.NotNil(t, generator.Providers[defaultProviderID])
				return generator
			},
		},
		{
			name: "should generate config with success even with cloudfront disabled",
			setup: func(ctrl *gomock.Controller) *ConfigGenerator {
				configGenerator := NewConfigGenerator()
				configGenerator.Providers["cloudfront"].Enabled = false
				configGenerator.Providers["akamai"].Enabled = true

				verifier := NewMockverifier(ctrl)
				verifier.EXPECT().Vet(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
				configGenerator.verifier = verifier

				certGrabber := NewMockcertGrabber(ctrl)
				certGrabber.EXPECT().GetCertificate(gomock.Any(), gomock.Any()).Return(cert, nil).AnyTimes()
				configGenerator.certGrabber = certGrabber

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
			givenDNSTTConfig:     dnsttCfg,
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
				assert.NotNil(t, globalConfig.Client.Fronted.Providers["akamai"].VerifyHostname)
			},
		},
		{
			name:                 "nil DNSTT config",
			setup:                defaultSetup,
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
				assert.Nil(t, globalConfig.DNSTTConfig, "DNSTT config should be nil when not provided")
			},
		},
		{
			name:                 "nil amp config",
			setup:                defaultSetup,
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
				assert.Nil(t, globalConfig.AMPCacheConfig, "AMP cache config should be nil when not provided")
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			generator := tt.setup(ctrl)
			cfg, err := generator.GenerateConfig(
				tt.givenContext,
				tt.givenTemplate,
				tt.givenMasquerades,
				tt.givenProxiedSites,
				tt.givenBlacklist,
				tt.givenNumberOfWorkers,
				tt.givenMinFrequency,
				tt.givenMinMasquerades,
				tt.givenMaxMasquerades,
				tt.givenDNSTTConfig,
				tt.givenAMPCacheConfig,
			)
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
