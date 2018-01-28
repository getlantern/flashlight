package config

import (
	"strings"
)

// AdOptions are settings to use when showing ads to Android clients
type AdSettings struct {
	ShowAds      bool
	Percentage   float64
	Provider     string
	TargetedApps map[string]string
	Admob        *Admob
	InMobi       *InMobi
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
func (cfg *Global) ShowAds() bool {
	if cfg.AdSettings != nil {
		return cfg.AdSettings.ShowAds
	}
	return false
}

func (cfg *Global) Provider() string {
	if cfg.AdSettings != nil {
		return cfg.AdSettings.Provider
	}
	return ""
}

func (cfg *Global) Percentage() float64 {
	if cfg.AdSettings != nil {
		return cfg.AdSettings.Percentage
	}
	return 0
}

// targettedApps returns the apps to show splash screen ads for
func (cfg *Global) TargetedApps(region string) string {
	if cfg.AdSettings != nil {
		return cfg.AdSettings.TargetedApps[region]
	}
	return ""
}

func (cfg *Global) AppId() string {
	if cfg.AdSettings != nil && cfg.AdSettings.Admob != nil {
		log.Debugf("Admob id is %s", cfg.AdSettings.Admob.AppId)
		return cfg.AdSettings.Admob.AppId
	}
	return ""
}

func (cfg *Global) AdunitId() string {
	if cfg.AdSettings != nil && cfg.AdSettings.Admob != nil {
		return cfg.AdSettings.Admob.AdunitId
	}
	return ""
}

func (cfg *Global) InterstitialAdId() string {
	if cfg.AdSettings == nil {
		return ""
	}
	if strings.EqualFold(cfg.AdSettings.Provider, "inmobi") {
		return cfg.AdSettings.InMobi.InterstitialAdId
	} else {
		return cfg.AdSettings.Admob.InterstitialAdId
	}
}

func (cfg *Global) NativeAdId() string {
	if cfg.AdSettings != nil && cfg.AdSettings.InMobi != nil {
		return cfg.AdSettings.InMobi.NativeAdId
	}
	return ""
}

func (cfg *Global) VideoAdunitId() string {
	if cfg.AdSettings != nil && cfg.AdSettings.Admob != nil {
		return cfg.AdSettings.Admob.VideoAdunitId
	}
	return ""
}
