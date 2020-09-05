package simbrowser

import (
	"testing"

	"github.com/getlantern/yaml"
	"github.com/stretchr/testify/require"
)

func TestMarketShareDataYamlRoundTrip(t *testing.T) {
	msd := MarketShareData{
		Chrome:           0.4,
		Firefox:          0.3,
		Edge:             0.2,
		InternetExplorer: 0.1,
	}

	b, err := yaml.Marshal(msd)
	require.NoError(t, err)
	roundTripped := MarketShareData{}
	require.NoError(t, yaml.Unmarshal(b, &roundTripped))
	require.Equal(t, msd, roundTripped)
}
