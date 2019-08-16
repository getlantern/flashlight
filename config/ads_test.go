package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdSettings(t *testing.T) {
	s := &AdSettings{
		NativeBannerZoneID:   "a",
		StandardBannerZoneID: "b",
		InterstitialZoneID:   "c",
		DaysToSuppress:       1,
		Percentage:           100,
		Countries: map[string]string{
			"ir": "pro",
			"ae": "free",
			"us": "none",
			"uk": "wrong",
		},
	}

	assert.True(t, s.adsEnabled(true, "IR", 1))
	assert.True(t, s.adsEnabled(false, "IR", 1))
	assert.False(t, s.adsEnabled(true, "AE", 1))
	assert.True(t, s.adsEnabled(false, "AE", 1))
	assert.False(t, s.adsEnabled(false, "AE", 0))
	assert.False(t, s.adsEnabled(false, "US", 1))
	assert.False(t, s.adsEnabled(false, "UK", 1))
	assert.False(t, s.adsEnabled(false, "Ru", 1))

	p := s.GetAdProvider(false, "IR", 1)
	if assert.NotNil(t, p) {
		if assert.True(t, p.ShouldShowAd()) {
			assert.Equal(t, s.NativeBannerZoneID, p.GetNativeBannerZoneID())
			assert.Equal(t, s.StandardBannerZoneID, p.GetStandardBannerZoneID())
			assert.Equal(t, s.InterstitialZoneID, p.GetInterstitialZoneID())
		}
	}
}
