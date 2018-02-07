package config

import (
	"strings"
)

// AdSettings are settings to use when showing ads to Android clients
type AdSettings struct {
	ShowAds        bool
	MinDaysShowAds int `yaml:"mindaysshowads,omitempty"`
	MaxDaysShowAds int `yaml:"maxdaysshowads,omitempty"`
	Percentage     float64
	Provider       string
	TargetedApps   map[string]string
	Admob          *Admob
	InMobi         *InMobi
}

type Admob struct {
	AppId            string
	AdunitId         string
	InterstitialAdId string
	VideoAdunitId    string
}

type InMobi struct {
	AppId            string
	InterstitialAdId string
	NativeAdId       string
}

// showAds is a global indicator to show ads to clients at all
func (settings *AdSettings) Enabled() bool {
	if settings != nil {
		return settings.ShowAds
	}
	return false
}

func (settings *AdSettings) GetMinDaysShowAds() int {
	if settings != nil {
		return settings.MinDaysShowAds
	}
	return 0
}

func (settings *AdSettings) GetMaxDaysShowAds() int {
	if settings != nil {
		return settings.MaxDaysShowAds
	}
	return 0
}

func (settings *AdSettings) GetProvider() string {
	if settings != nil {
		return settings.Provider
	}
	return ""
}

func (settings *AdSettings) GetPercentage() float64 {
	if settings != nil {
		return settings.Percentage
	}
	return 0
}

// targettedApps returns the apps to show splash screen ads for
func (settings *AdSettings) GetTargetedApps(region string) string {
	if settings != nil {
		return settings.TargetedApps[region]
	}
	return ""
}

func (settings *AdSettings) AppId() string {
	if settings != nil && settings.Admob != nil {
		log.Debugf("Admob id is %s", settings.Admob.AppId)
		return settings.Admob.AppId
	}
	return ""
}

func (settings *AdSettings) AdunitId() string {
	if settings != nil && settings.Admob != nil {
		return settings.Admob.AdunitId
	}
	return ""
}

func (settings *AdSettings) InterstitialAdId() string {
	if settings == nil {
		return ""
	}
	if strings.EqualFold(settings.Provider, "inmobi") {
		return settings.InMobi.InterstitialAdId
	} else {
		return settings.Admob.InterstitialAdId
	}
}

func (settings *AdSettings) NativeAdId() string {
	if settings != nil && settings.InMobi != nil {
		return settings.InMobi.NativeAdId
	}
	return ""
}

func (settings *AdSettings) VideoAdunitId() string {
	if settings != nil && settings.Admob != nil {
		return settings.Admob.VideoAdunitId
	}
	return ""
}
