package config

// AdOptions are settings to use when showing ads to Android clients
type AdSettings struct {
	ShowAds bool
	TargettedApps map[string]string
	Admob *Admob
}

type Admob struct {
	AppId string
	AdUnitId string
	InterstitialAdId string
	VideoAdUnitId string
}

// showAds is a global indicator to show ads to clients at all
func (cfg *Global) ShowAds() bool {
	if cfg.AdSettings != nil {
		return cfg.AdSettings.ShowAds
	}
	return false
}

// targettedApps returns the apps to show splash screen ads for
func (cfg *Global) TargettedApps(region string) string {
	if cfg.AdSettings != nil {
		return cfg.AdSettings.TargettedApps[region]
	}
	return ""
}

func (cfg *Global) AppId() string {
	if cfg.AdSettings != nil && cfg.AdSettings.Admob != nil {
		return cfg.AdSettings.Admob.AppId
	}
	return ""
}

func (cfg *Global) AdUnitId() string {
	if cfg.AdSettings != nil && cfg.AdSettings.Admob != nil {
		return cfg.AdSettings.Admob.AdUnitId
	}
	return ""
}

func (cfg *Global) InterstitialAdId() string {
	if cfg.AdSettings != nil && cfg.AdSettings.Admob != nil {
		return cfg.AdSettings.Admob.InterstitialAdId
	}
	return ""
}

func (cfg *Global) VideoAdUnitId() string {
	if cfg.AdSettings != nil && cfg.AdSettings.Admob != nil {
		return cfg.AdSettings.Admob.VideoAdUnitId
	}
	return ""
}