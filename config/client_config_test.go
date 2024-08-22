package config

import (
	"testing"

	"github.com/getlantern/fronted"
	"github.com/stretchr/testify/assert"
)

func TestFrontedProviders(t *testing.T) {
	verifyHostname := "verifyHostname.com"
	var tests = []struct {
		name              string
		givenClientConfig *ClientConfig
		assert            func(t *testing.T, providersMap map[string]*fronted.Provider)
	}{
		{
			name:              "empty client config should return a empty providers map",
			givenClientConfig: NewClientConfig(),
			assert: func(t *testing.T, providersMap map[string]*fronted.Provider) {
				assert.Equal(t, 0, len(providersMap))
			},
		},
		{
			name: "client config with one provider should return a map with one provider and its config",
			givenClientConfig: &ClientConfig{
				Fronted: &FrontedConfig{
					Providers: map[string]*ProviderConfig{
						"provider1": {
							HostAliases: map[string]string{
								"host1": "alias1",
							},
							TestURL: "testURL",
							Masquerades: []*fronted.Masquerade{
								{
									Domain:    "domain1",
									IpAddress: "127.0.0.1",
								},
							},
							Validator: &ValidatorConfig{
								RejectStatus: []int{404},
							},
							PassthroughPatterns: []string{"pattern1"},
							VerifyHostname:      &verifyHostname,
							FrontingSNIs: map[string]*fronted.SNIConfig{
								"default": {
									UseArbitrarySNIs: false,
									ArbitrarySNIs:    []string{"sni1"},
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, providersMap map[string]*fronted.Provider) {
				assert.Equal(t, 1, len(providersMap))
				provider1 := providersMap["provider1"]
				assert.Equal(t, map[string]string{"host1": "alias1"}, provider1.HostAliases)
				assert.Equal(t, "testURL", provider1.TestURL)
				assert.Equal(t, fronted.Masquerade{
					Domain:         "domain1",
					IpAddress:      "127.0.0.1",
					VerifyHostname: &verifyHostname,
					SNI:            "sni1",
				}, *provider1.Masquerades[0])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providersMap := tt.givenClientConfig.FrontedProviders()
			tt.assert(t, providersMap)
		})
	}
}
