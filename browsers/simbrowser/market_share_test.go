package simbrowser

import (
	"testing"

	"github.com/getlantern/common"
	"github.com/getlantern/yaml"
	"github.com/stretchr/testify/require"
)

func TestMarketShareDataYamlRoundTrip(t *testing.T) {
	msd := common.MarketShareData{
		Chrome:           0.4,
		Firefox:          0.3,
		Edge:             0.2,
		InternetExplorer: 0.1,
	}

	b, err := yaml.Marshal(msd)
	require.NoError(t, err)
	roundTripped := common.MarketShareData{}
	require.NoError(t, yaml.Unmarshal(b, &roundTripped))
	require.Equal(t, msd, roundTripped)
}

func TestSetMarketShareData(t *testing.T) {
	validGlobal := common.MarketShareData{
		Chrome: 1.00,
	}
	validRegional := map[CountryCode]common.MarketShareData{
		"CN": {
			Firefox: 1.00,
		},
	}

	expectedMarketShareData := map[CountryCode][]browserChoice{
		globally: {
			{chrome, 1},
		},
		"CN": {
			{firefox, 1},
		},
	}

	require.NoError(t, SetMarketShareData(validGlobal, validRegional))
	require.Equal(t, expectedMarketShareData, marketShareData)

	invalidGlobal := common.MarketShareData{}
	require.Errorf(t, SetMarketShareData(invalidGlobal, validRegional), "SetMarketShareData must not allow an empty global parameter")
}
