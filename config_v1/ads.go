package config

import (
	"math/rand"
	"strings"
)

const (
	none = "none"
	free = "free"
	pro  = "pro"
)

// AdSettings are settings to use when showing ads to Android clients
type AdSettings struct {
	NativeBannerZoneID   string `yaml:"nativebannerzoneid,omitempty"`
	StandardBannerZoneID string `yaml:"standardbannerzoneid,omitempty"`
	InterstitialZoneID   string `yaml:"interstitialzoneid,omitempty"`
	DaysToSuppress       int    `yaml:"daystosuppress,omitempty"`
	Percentage           float64
	Countries            map[string]string
}

type AdProvider struct {
	AdSettings
}

func (s *AdSettings) GetAdProvider(isPro bool, countryCode string, daysSinceInstalled int) *AdProvider {
	if !s.adsEnabled(isPro, countryCode, daysSinceInstalled) {
		return nil
	}

	return &AdProvider{*s}
}

func (s *AdSettings) adsEnabled(isPro bool, countryCode string, daysSinceInstalled int) bool {
	if s == nil {
		return false
	}

	if daysSinceInstalled < s.DaysToSuppress {
		return false
	}

	level := s.Countries[strings.ToLower(countryCode)]
	switch level {
	case free:
		return !isPro
	case pro:
		return true
	default:
		return false
	}
}

func (p *AdProvider) GetNativeBannerZoneID() string {
	if p == nil {
		return ""
	}
	return p.NativeBannerZoneID
}

func (p *AdProvider) GetStandardBannerZoneID() string {
	if p == nil {
		return ""
	}
	return p.StandardBannerZoneID
}

func (p *AdProvider) GetInterstitialZoneID() string {
	if p == nil {
		return ""
	}
	return p.InterstitialZoneID
}

func (p *AdProvider) ShouldShowAd() bool {
	return rand.Float64() <= p.Percentage/100
}
