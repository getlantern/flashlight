package flashlight

import (
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"time"

	commonconfig "github.com/getlantern/common/config"
	"github.com/getlantern/detour"
	"github.com/getlantern/dhtup"
	"github.com/getlantern/dnsgrab"
	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/fronted"
	"github.com/getlantern/golog"
	"github.com/getlantern/netx"
	"github.com/getlantern/ops"
	"github.com/getlantern/proxybench"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/borda"
	"github.com/getlantern/flashlight/broflake"
	"github.com/getlantern/flashlight/bypass"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/domainrouting"
	"github.com/getlantern/flashlight/email"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/goroutines"
	"github.com/getlantern/flashlight/logging"
	fops "github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/otel"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/shortcut"
	"github.com/getlantern/flashlight/stats"
	p2pLogger "github.com/getlantern/libp2p/logger"
	"github.com/getlantern/quicproxy"
)

var (
	log              = golog.LoggerFor("flashlight")
	quicProxyLogger  = golog.LoggerFor("flashlight.quicproxy")
	replicaP2pLogger = golog.LoggerFor("flashlight.libp2p")

	startProxyBenchOnce sync.Once

	// blockingRelevantFeatures lists all features that might affect blocking and gives their
	// default enabled status (until we know the country)
	blockingRelevantFeatures = map[string]bool{
		config.FeatureProxyBench:           false,
		config.FeatureGoogleSearchAds:      false,
		config.FeatureNoBorda:              true,
		config.FeatureProbeProxies:         false,
		config.FeatureDetour:               false,
		config.FeatureNoHTTPSEverywhere:    true,
		config.FeatureProxyWhitelistedOnly: true,
	}
)

func init() {
	netx.EnableNAT64AutoDiscovery()

	// Init logger for different packages
	quicproxy.Log = logging.FlashlightLogger{Logger: quicProxyLogger}
	p2pLogger.Log = logging.FlashlightLogger{Logger: replicaP2pLogger}
}

// HandledErrorType is used to differentiate error types to handlers configured via
// Flashlight.SetErrorHandler.
type HandledErrorType int

const (
	ErrorTypeProxySaveFailure  HandledErrorType = iota
	ErrorTypeConfigSaveFailure HandledErrorType = iota
)

func (t HandledErrorType) String() string {
	switch t {
	case ErrorTypeProxySaveFailure:
		return "proxy save failure"
	case ErrorTypeConfigSaveFailure:
		return "config save failure"
	default:
		return fmt.Sprintf("unrecognized error type %d", t)
	}
}

type ProxyListener interface {
	OnProxies(map[string]*commonconfig.ProxyConfig)
}

type Flashlight struct {
	configDir         string
	flagsAsMap        map[string]interface{}
	userConfig        common.UserConfig
	isPro             func() bool
	mxGlobal          sync.RWMutex
	global            *config.Global
	onProxiesUpdate   func([]balancer.Dialer, config.Source)
	onConfigUpdate    func(*config.Global, config.Source)
	onBordaConfigured chan bool
	autoReport        func() bool
	client            *client.Client
	op                *fops.Op
	errorHandler      func(HandledErrorType, error)
	dhtupContext      *dhtup.Context
	mxProxyListeners  sync.RWMutex
	proxyListeners    []func(map[string]*commonconfig.ProxyConfig, config.Source)
}

func (f *Flashlight) onGlobalConfig(cfg *config.Global, src config.Source) {
	log.Debugf("Got global config from %v", src)
	f.mxGlobal.Lock()
	f.global = cfg
	f.mxGlobal.Unlock()
	domainrouting.Configure(cfg.DomainRoutingRules, cfg.ProxiedSites)
	f.applyClientConfig(cfg)
	f.applyProxyBench(cfg)
	f.applyBroflake(cfg)
	f.applyProxiedFlow(cfg)
	f.applyBorda(cfg)
	f.applyOtel(cfg)
	select {
	case f.onBordaConfigured <- true:
		// okay
	default:
		// ignore
	}
	f.onConfigUpdate(cfg, src)
	f.reconfigureGoogleAds()
}

func (f *Flashlight) reconfigureGoogleAds() {
	var opts config.GoogleSearchAdsOptions
	if err := f.FeatureOptions(config.FeatureGoogleSearchAds, &opts); err == nil {
		f.client.ConfigureGoogleAds(opts)
	} else {
		log.Errorf("Unable to configure google search ads: %v", err)
	}
}

// EnabledFeatures gets all features enabled based on current conditions
func (f *Flashlight) EnabledFeatures() map[string]bool {
	featuresEnabled := make(map[string]bool)
	f.mxGlobal.RLock()
	if f.global == nil {
		f.mxGlobal.RUnlock()
		return featuresEnabled
	}
	global := f.global
	f.mxGlobal.RUnlock()
	country := geolookup.GetCountry(0)
	for feature := range global.FeaturesEnabled {
		if f.calcFeature(global, country, feature) {
			featuresEnabled[feature] = true
		}
	}
	return featuresEnabled
}

// EnableNamedDomainRules adds named domain rules specified as arguments to the domainrouting rules table
func (f *Flashlight) EnableNamedDomainRules(names ...string) {
	f.mxGlobal.RLock()
	global := f.global
	f.mxGlobal.RUnlock()
	if global == nil {
		return
	}
	for _, name := range names {
		if v, ok := global.NamedDomainRoutingRules[name]; ok {
			if err := domainrouting.AddRules(v); err != nil {
				_ = log.Errorf("Unable to add named domain routing rules: %v", err)
			}
		} else {
			log.Debugf("Named domain routing rule %s is not defined in global config", name)
		}
	}
}

// DisableNamedDomainRules removes named domain rules specified as arguments from the domainrouting rules table
func (f *Flashlight) DisableNamedDomainRules(names ...string) {
	f.mxGlobal.RLock()
	global := f.global
	f.mxGlobal.RUnlock()
	if global == nil {
		return
	}
	for _, name := range names {
		if v, ok := global.NamedDomainRoutingRules[name]; !ok {
			if err := domainrouting.RemoveRules(v); err != nil {
				_ = log.Errorf("Unable to remove named domain routing rules: %v", err)
			}
		} else {
			log.Debugf("Named domain routing rule %s is not defined in global config", name)
		}
	}
}

// FeatureEnabled returns true if the input feature is enabled for this flashlight instance. Feature
// names are tracked in the config package.
func (f *Flashlight) FeatureEnabled(feature string) bool {
	f.mxGlobal.RLock()
	global := f.global
	f.mxGlobal.RUnlock()
	return f.calcFeature(global, geolookup.GetCountry(0), feature)
}

func (f *Flashlight) calcFeature(global *config.Global, country, feature string) bool {
	// Special case: Use defaults for blocking related features until geolookup is finished
	// to avoid accidentally generating traffic that could trigger blocking.
	enabled, blockingRelated := blockingRelevantFeatures[feature]
	if country == "" && blockingRelated {
		enabledText := "disabled"
		if enabled {
			enabledText = "enabled"
		}
		log.Debugf("Blocking related feature %v %v because geolookup has not yet finished", feature, enabledText)
		return enabled
	}
	if global == nil {
		log.Error("No global configuration!")
		return enabled
	}
	return global.FeatureEnabled(feature,
		common.Platform,
		f.userConfig.GetAppName(),
		common.Version,
		f.userConfig.GetUserID(),
		f.isPro(),
		country)
}

// FeatureOptions unmarshals options for the input feature. Feature names are tracked in the config
// package.
func (f *Flashlight) FeatureOptions(feature string, opts config.FeatureOptions) error {
	f.mxGlobal.RLock()
	global := f.global
	f.mxGlobal.RUnlock()
	if global == nil {
		// just to be safe
		return errors.New("No global configuration")
	}
	return global.UnmarshalFeatureOptions(feature, opts)
}

func (f *Flashlight) addProxyListener(listener func(proxies map[string]*commonconfig.ProxyConfig, src config.Source)) {
	f.mxProxyListeners.Lock()
	defer f.mxProxyListeners.Unlock()
	f.proxyListeners = append(f.proxyListeners, listener)
}

func (f *Flashlight) notifyProxyListeners(proxies map[string]*commonconfig.ProxyConfig, src config.Source) {
	f.mxProxyListeners.RLock()
	defer f.mxProxyListeners.RUnlock()
	for _, l := range f.proxyListeners {
		// Make absolutely sure we don't hit data races with different go routines
		// accessing shared data -- give each go routine it's own copy.
		proxiesCopy := chained.CopyConfigs(proxies)
		go l(proxiesCopy, src)
	}
}

func (f *Flashlight) startConfigFetch() func() {
	proxiesDispatch := func(conf interface{}, src config.Source) {
		proxyMap := conf.(map[string]*commonconfig.ProxyConfig)
		f.notifyProxyListeners(proxyMap, src)
	}
	globalDispatch := func(conf interface{}, src config.Source) {
		cfg := conf.(*config.Global)
		log.Debugf("Applying global config")
		f.onGlobalConfig(cfg, src)
	}
	rt := &proxied.MaybeProxiedFlowRoundTripper{
		Default: proxied.ParallelPreferChained(),
		Flow: proxied.NewProxiedFlow(
			&proxied.ProxiedFlowOptions{
				ParallelMethods: proxied.AnyMethod,
			}).Add(proxied.FlowComponentID_Chained, true).
			Add(proxied.FlowComponentID_Fronted, false).
			Add(proxied.FlowComponentID_Broflake, false)}

	onProxiesSaveError := func(err error) {
		f.errorHandler(ErrorTypeProxySaveFailure, err)
	}
	onConfigSaveError := func(err error) {
		f.errorHandler(ErrorTypeConfigSaveFailure, err)
	}

	stopConfig := config.Init(
		f.configDir, f.flagsAsMap, f.userConfig,
		proxiesDispatch, onProxiesSaveError,
		globalDispatch, onConfigSaveError, rt, f.dhtupContext)
	return stopConfig
}

func (f *Flashlight) applyProxyBench(cfg *config.Global) {
	go func() {
		// Wait a while for geolookup to happen before checking if we can turn on proxybench
		geolookup.GetCountry(1 * time.Minute)
		if f.FeatureEnabled(config.FeatureProxyBench) {
			startProxyBenchOnce.Do(func() {
				opts := &proxybench.Opts{
					UpdateURL: "https://s3.amazonaws.com/lantern/proxybench.json",
				}
				proxybench.Start(opts, func(timing time.Duration, ctx map[string]interface{}) {})
			})
		} else {
			log.Debug("proxybench disabled")
		}
	}()
}

func (f *Flashlight) applyBorda(cfg *config.Global) {
	_enableBorda := borda.Enabler(cfg.BordaSamplePercentage)
	enableBorda := func(ctx map[string]interface{}) bool {
		if !f.autoReport() {
			// User has chosen not to automatically submit data
			return false
		}

		if f.FeatureEnabled(config.FeatureNoBorda) {
			// Borda is disabled by global config
			return false
		}

		return _enableBorda(ctx)
	}
	borda.Configure(cfg.BordaReportInterval, enableBorda)
}

func (f *Flashlight) applyOtel(cfg *config.Global) {
	if cfg.Otel != nil {
		for _, op := range []string{"fetch_config", "proxiedflow", "proxiedflowcomponent"} {
			log.Debugf("Overriding default otel sample rate for op %v (%v)", op, 1)
			cfg.Otel.OpSampleRates[op] = 1
		}
		otel.Configure(cfg.Otel)
	}
}

func (f *Flashlight) applyBroflake(cfg *config.Global) {

	fopts := &config.BroflakeOptions{
		DiscoverySrv:   "https://broflake-freddie-xdy27ofj3a-ue.a.run.app",
		Endpoint:       "/v1/signal",
		GenesisAddr:    "genesis",
		NATFailTimeout: 5 * time.Second,
		ICEFailTimeout: 5 * time.Second,
		STUNBatchSize:  5,
		STUNSrvs: []string{
			"stun:193.16.148.245:3478",
			"stun:176.9.24.184:3478",
			"stun:77.72.169.212:3478",
			"stun:91.122.224.102:3478",
			"stun:77.72.169.213:3478",
			"stun:188.123.97.201:3478",
			"stun:142.250.21.127:19302",
			"stun:77.72.169.213:3478",
			"stun:185.18.24.50:3478",
			"stun:198.72.119.88:3478",
			"stun:185.67.224.59:3478",
			"stun:147.182.188.245:3478",
			"stun:185.18.24.50:3478",
			"stun:185.112.247.26:3478",
			"stun:195.145.93.141:3478",
			"stun:115.126.160.126:3478",
			"stun:212.227.67.34:3478",
			"stun:54.223.41.250:3478",
			"stun:193.22.2.248:3478",
			"stun:157.22.130.80:3478",
			"stun:217.146.224.74:3478",
			"stun:89.106.220.34:3478",
			"stun:77.72.169.210:3478",
			"stun:85.93.219.114:3478",
			"stun:5.161.52.174:3478",
			"stun:204.197.144.2:3478",
			"stun:5.178.34.84:3478",
			"stun:185.18.24.50:3478",
			"stun:204.197.159.2:3478",
			"stun:154.73.34.8:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.211:3478",
			"stun:84.198.248.217:3478",
			"stun:217.10.68.145:10000",
			"stun:182.154.16.5:3478",
			"stun:51.83.15.212:3478",
			"stun:94.75.247.45:3478",
			"stun:82.113.193.63:3478",
			"stun:77.72.169.212:3478",
			"stun:212.227.67.33:3478",
			"stun:202.49.164.49:3478",
			"stun:5.9.87.18:3478",
			"stun:104.130.210.14:3478",
			"stun:217.0.136.1:3478",
			"stun:185.18.24.50:3478",
			"stun:77.72.169.211:3478",
			"stun:80.250.1.173:3478",
			"stun:77.72.169.212:3478",
			"stun:77.72.169.213:3478",
			"stun:54.197.117.0:3478",
			"stun:77.72.169.210:3478",
			"stun:202.1.117.2:3478",
			"stun:77.72.169.211:3478",
			"stun:85.197.87.182:3478",
			"stun:143.198.60.79:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.213:3478",
			"stun:5.161.52.174:3478",
			"stun:3.208.102.142:3478",
			"stun:185.39.86.17:3478",
			"stun:92.63.111.151:3478",
			"stun:77.72.169.212:3478",
			"stun:34.197.205.39:3478",
			"stun:77.72.169.212:3478",
			"stun:212.144.246.197:3478",
			"stun:217.119.210.45:3478",
			"stun:217.10.68.145:3478",
			"stun:195.35.115.37:3478",
			"stun:83.125.8.47:3478",
			"stun:95.216.145.84:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.212:3478",
			"stun:209.105.241.31:3478",
			"stun:77.72.169.210:3478",
			"stun:74.125.24.127:19305",
			"stun:63.211.239.133:3478",
			"stun:195.242.206.1:3478",
			"stun:77.72.169.211:3478",
			"stun:157.161.10.32:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.211:3478",
			"stun:146.185.186.157:3478",
			"stun:77.72.169.212:3478",
			"stun:185.18.24.50:3478",
			"stun:5.9.87.18:3478",
			"stun:212.80.226.18:3478",
			"stun:216.93.246.18:3478",
			"stun:137.74.112.113:3478",
			"stun:217.0.12.17:3478",
			"stun:77.72.169.212:3478",
			"stun:24.204.48.11:3478",
			"stun:34.74.124.204:3478",
			"stun:77.72.169.211:3478",
			"stun:185.41.24.10:3478",
			"stun:195.35.114.37:3478",
			"stun:77.72.169.212:3478",
			"stun:37.59.92.57:3478",
			"stun:66.110.73.74:3478",
			"stun:77.72.169.212:3478",
			"stun:62.138.0.157:3478",
			"stun:185.18.24.50:3478",
			"stun:13.59.93.103:3478",
			"stun:83.64.250.246:3478",
			"stun:23.253.102.137:3478",
			"stun:82.113.193.63:3478",
			"stun:77.72.169.211:3478",
			"stun:52.26.251.34:3478",
			"stun:88.198.203.20:3478",
			"stun:217.0.11.225:3478",
			"stun:217.91.243.229:3478",
			"stun:194.169.214.30:3478",
			"stun:178.33.166.29:3478",
			"stun:192.172.233.145:3478",
			"stun:195.35.115.37:3478",
			"stun:23.252.81.20:3478",
			"stun:88.86.102.51:3478",
			"stun:209.250.250.224:3478",
			"stun:64.131.63.216:3478",
			"stun:77.72.169.212:3478",
			"stun:54.183.232.212:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.212:3478",
			"stun:212.227.67.34:3478",
			"stun:77.72.169.212:3478",
			"stun:52.24.174.49:3478",
			"stun:77.72.169.211:3478",
			"stun:20.93.239.167:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.211:3478",
			"stun:92.222.127.114:3478",
			"stun:77.72.169.213:3478",
			"stun:94.130.130.49:3478",
			"stun:77.72.169.210:3478",
			"stun:157.90.156.59:3478",
			"stun:193.22.119.20:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.212:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.210:3478",
			"stun:70.42.198.34:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.213:3478",
			"stun:212.29.18.56:3478",
			"stun:77.72.169.212:3478",
			"stun:194.87.0.22:3478",
			"stun:31.13.72.3:3478",
			"stun:212.227.67.34:3478",
			"stun:77.72.169.213:3478",
			"stun:31.184.236.23:3478",
			"stun:217.10.68.145:3478",
			"stun:52.76.91.67:3478",
			"stun:77.72.169.212:3478",
			"stun:91.217.201.14:3478",
			"stun:88.198.203.20:3478",
			"stun:94.23.17.185:3478",
			"stun:185.39.86.17:3478",
			"stun:142.93.228.31:3478",
			"stun:81.19.224.87:3478",
			"stun:77.72.169.211:3478",
			"stun:216.93.246.18:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.212:3478",
			"stun:74.125.204.127:19302",
			"stun:77.243.0.75:3478",
			"stun:212.101.4.120:3478",
			"stun:108.177.14.127:19305",
			"stun:185.158.144.41:3478",
			"stun:216.93.246.18:3478",
			"stun:91.212.41.85:3478",
			"stun:216.93.246.18:3478",
			"stun:77.72.169.211:3478",
			"stun:23.21.92.55:3478",
			"stun:77.237.51.83:3478",
			"stun:77.72.169.210:3478",
			"stun:212.227.67.33:3478",
			"stun:216.228.192.76:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.212:3478",
			"stun:91.205.60.185:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.210:3478",
			"stun:83.211.9.232:3478",
			"stun:34.193.110.91:3478",
			"stun:81.23.228.150:3478",
			"stun:51.68.45.75:3478",
			"stun:216.93.246.18:3478",
			"stun:81.91.111.11:3478",
			"stun:18.194.146.65:3478",
			"stun:77.72.169.210:3478",
			"stun:83.211.9.232:3478",
			"stun:212.227.67.34:3478",
			"stun:77.72.169.211:3478",
			"stun:213.140.209.236:3478",
			"stun:94.130.130.49:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.210:3478",
			"stun:87.129.12.229:3478",
			"stun:217.10.68.152:3478",
			"stun:212.25.7.87:3478",
			"stun:77.72.169.213:3478",
			"stun:81.3.27.44:3478",
			"stun:212.227.67.33:3478",
			"stun:83.211.9.232:3478",
			"stun:77.72.169.212:3478",
			"stun:188.68.43.182:3478",
			"stun:35.180.81.93:3478",
			"stun:108.177.14.127:19302",
			"stun:77.72.169.212:3478",
			"stun:77.72.169.213:3478",
			"stun:83.64.250.246:3478",
			"stun:77.72.169.212:3478",
			"stun:213.157.4.53:3478",
			"stun:185.18.24.50:3478",
			"stun:121.101.91.194:3478",
			"stun:85.17.88.164:3478",
			"stun:185.18.24.50:3478",
			"stun:83.211.9.232:3478",
			"stun:77.72.169.213:3478",
			"stun:188.68.43.182:3478",
			"stun:91.121.209.194:3478",
			"stun:217.10.68.152:3478",
			"stun:81.83.12.46:3478",
			"stun:77.72.169.213:3478",
			"stun:90.145.158.66:3478",
			"stun:162.13.119.185:3478",
			"stun:193.22.17.97:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.212:3478",
			"stun:182.154.16.7:3478",
			"stun:82.113.193.63:3478",
			"stun:136.243.59.79:3478",
			"stun:185.18.24.50:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.212:3478",
			"stun:77.72.169.210:3478",
			"stun:91.213.98.54:3478",
			"stun:216.93.246.18:3478",
			"stun:217.10.68.152:10000",
			"stun:77.72.169.212:3478",
			"stun:77.72.169.211:3478",
			"stun:62.96.96.137:3478",
			"stun:77.72.169.212:3478",
			"stun:212.227.67.33:3478",
			"stun:209.242.17.106:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.212:3478",
			"stun:77.72.169.211:3478",
			"stun:159.69.191.124:3478",
			"stun:23.21.92.55:3478",
			"stun:77.72.169.213:3478",
			"stun:139.59.84.212:3478",
			"stun:5.161.52.174:3478",
			"stun:77.72.169.211:3478",
			"stun:101.200.40.149:3478",
			"stun:212.227.67.34:3478",
			"stun:77.72.169.213:3478",
			"stun:45.15.102.31:3478",
			"stun:185.88.236.76:3478",
			"stun:77.72.169.211:3478",
			"stun:34.73.197.25:3478",
			"stun:77.72.169.211:3478",
			"stun:159.69.191.124:443",
			"stun:158.69.57.20:3478",
			"stun:217.74.179.29:3478",
			"stun:91.199.161.149:3478",
			"stun:212.18.0.14:3478",
			"stun:212.227.67.33:3478",
			"stun:216.93.246.18:3478",
			"stun:217.10.68.145:3478",
			"stun:217.10.68.145:3478",
			"stun:52.26.251.34:3478",
			"stun:136.243.59.79:3478",
			"stun:91.121.128.132:3478",
			"stun:77.72.169.213:3478",
			"stun:185.140.24.140:3478",
			"stun:77.72.169.212:3478",
			"stun:192.76.120.66:3478",
			"stun:185.18.24.50:3478",
			"stun:77.72.169.212:3478",
			"stun:202.49.164.50:3478",
			"stun:217.10.68.145:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.210:3478",
			"stun:111.206.174.2:3478",
			"stun:212.103.200.6:3478",
			"stun:77.72.169.211:3478",
			"stun:185.18.24.50:3478",
			"stun:77.72.169.213:3478",
			"stun:178.63.107.149:3478",
			"stun:44.224.155.217:3478",
			"stun:91.151.52.200:3478",
			"stun:212.227.67.34:3478",
			"stun:217.10.68.152:3478",
			"stun:77.72.169.210:3478",
			"stun:212.45.38.40:3478",
			"stun:77.72.169.213:3478",
			"stun:161.53.1.100:3478",
			"stun:147.182.188.245:3478",
			"stun:217.0.12.1:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.212:3478",
			"stun:208.83.246.102:3478",
			"stun:193.43.148.37:3478",
			"stun:77.72.169.212:3478",
			"stun:217.10.68.152:3478",
			"stun:108.171.179.113:3478",
			"stun:212.227.67.34:3478",
			"stun:185.39.86.17:3478",
			"stun:194.61.59.30:3478",
			"stun:213.239.212.105:3478",
			"stun:194.140.246.192:3478",
			"stun:5.161.57.75:3478",
			"stun:77.72.169.212:3478",
			"stun:77.72.169.211:3478",
			"stun:185.88.7.40:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.211:3478",
			"stun:212.103.68.7:3478",
			"stun:23.21.199.62:3478",
			"stun:77.72.169.213:3478",
			"stun:185.18.24.50:3478",
			"stun:185.18.24.50:3478",
			"stun:79.140.42.88:3478",
			"stun:109.69.177.38:3478",
			"stun:129.153.212.128:3478",
			"stun:194.61.59.25:3478",
			"stun:185.18.24.50:3478",
			"stun:66.228.54.23:3478",
			"stun:5.161.57.75:3478",
			"stun:217.10.68.152:3478",
			"stun:88.99.67.241:3478",
			"stun:195.35.114.37:3478",
			"stun:216.93.246.18:3478",
			"stun:77.72.169.210:3478",
			"stun:54.173.127.160:3478",
			"stun:77.72.169.213:3478",
			"stun:85.17.88.164:3478",
			"stun:173.255.200.200:3478",
			"stun:77.72.169.210:3478",
			"stun:77.246.29.197:3478",
			"stun:207.38.89.164:3478",
			"stun:109.68.96.189:3478",
			"stun:194.149.74.157:3478",
			"stun:77.72.169.213:3478",
			"stun:216.93.246.18:3478",
			"stun:81.187.30.115:3478",
			"stun:217.10.68.152:3478",
			"stun:92.205.106.161:3478",
			"stun:77.72.169.212:3478",
			"stun:81.82.206.117:3478",
			"stun:83.64.250.246:3478",
			"stun:188.118.52.172:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.213:3478",
			"stun:162.243.29.166:3478",
			"stun:5.161.52.174:3478",
			"stun:77.72.169.213:3478",
			"stun:37.9.136.90:3478",
			"stun:180.235.108.91:3478",
			"stun:81.187.30.115:3478",
			"stun:81.162.64.162:3478",
			"stun:34.206.168.53:3478",
			"stun:77.72.169.213:3478",
			"stun:195.209.116.72:3478",
			"stun:77.72.169.210:3478",
			"stun:54.176.195.118:3478",
			"stun:177.66.4.31:3478",
			"stun:77.72.169.212:3478",
			"stun:212.118.209.86:3478",
			"stun:77.72.169.212:3478",
			"stun:216.93.246.18:3478",
			"stun:111.206.174.3:3478",
			"stun:217.10.68.152:3478",
			"stun:46.193.255.81:3478",
			"stun:185.67.224.58:3478",
			"stun:194.169.214.30:3478",
			"stun:18.191.223.12:3478",
			"stun:185.125.180.70:3478",
			"stun:134.119.17.210:3478",
			"stun:77.72.169.211:3478",
			"stun:142.250.21.127:19305",
			"stun:146.185.186.157:3478",
			"stun:217.0.136.17:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.213:3478",
			"stun:217.0.11.241:3478",
			"stun:77.72.169.212:3478",
			"stun:37.59.92.57:3478",
			"stun:88.218.220.40:3478",
			"stun:217.10.68.152:3478",
			"stun:185.125.180.70:3478",
			"stun:193.28.184.4:3478",
			"stun:77.72.169.211:3478",
			"stun:83.211.9.232:3478",
			"stun:77.72.169.213:3478",
			"stun:188.138.90.169:3478",
			"stun:198.61.197.182:3478",
			"stun:212.227.67.33:3478",
			"stun:77.72.169.212:3478",
			"stun:27.111.15.93:3478",
			"stun:77.72.169.212:3478",
			"stun:5.161.57.75:3478",
			"stun:77.72.169.211:3478",
			"stun:44.230.252.214:3478",
			"stun:77.72.169.211:3478",
			"stun:216.93.246.18:3478",
			"stun:77.72.169.212:3478",
			"stun:188.64.120.28:3478",
			"stun:216.144.89.2:3478",
			"stun:95.110.198.3:3478",
			"stun:91.205.60.185:3478",
			"stun:65.17.128.101:3478",
			"stun:41.79.23.6:3478",
			"stun:172.217.213.127:19302",
			"stun:69.89.160.30:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.210:3478",
			"stun:81.25.228.2:3478",
			"stun:77.72.169.211:3478",
			"stun:178.63.240.148:3478",
			"stun:5.161.57.75:3478",
			"stun:85.17.186.7:3478",
			"stun:216.93.246.18:3478",
			"stun:78.40.125.40:3478",
			"stun:213.239.206.5:3478",
			"stun:134.2.17.14:3478",
			"stun:77.72.169.213:3478",
			"stun:185.41.24.6:3478",
			"stun:77.72.169.211:3478",
			"stun:74.125.204.127:19305",
			"stun:121.101.91.194:3478",
			"stun:51.68.112.203:3478",
			"stun:216.93.246.18:3478",
			"stun:77.72.169.213:3478",
			"stun:85.93.219.114:3478",
			"stun:37.139.120.14:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.212:3478",
			"stun:18.191.223.12:3478",
			"stun:77.72.169.213:3478",
			"stun:217.19.216.178:3478",
			"stun:213.251.48.147:3478",
			"stun:198.100.144.121:3478",
			"stun:77.72.169.210:3478",
			"stun:74.125.24.127:19302",
			"stun:77.72.169.213:3478",
			"stun:52.47.70.236:3478",
			"stun:80.155.54.123:3478",
			"stun:104.238.184.174:3478",
			"stun:94.140.180.141:3478",
			"stun:77.72.169.212:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.210:3478",
			"stun:51.83.201.84:3478",
			"stun:194.87.0.22:3478",
			"stun:87.253.140.133:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.212:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.212:3478",
			"stun:78.111.72.53:3478",
			"stun:216.144.89.2:3478",
			"stun:18.195.48.248:3478",
			"stun:37.97.65.52:3478",
			"stun:54.177.85.190:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.212:3478",
			"stun:69.20.59.115:3478",
			"stun:64.131.63.217:3478",
			"stun:195.211.238.18:3478",
			"stun:212.69.48.253:3478",
			"stun:49.12.125.53:3478",
			"stun:176.9.179.80:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.210:3478",
			"stun:208.83.246.100:3478",
			"stun:77.72.169.213:3478",
			"stun:131.153.146.5:3478",
			"stun:192.99.194.90:3478",
			"stun:212.227.67.33:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.212:3478",
			"stun:150.254.161.60:3478",
			"stun:83.64.250.246:3478",
			"stun:65.99.199.231:3478",
			"stun:104.130.214.5:3478",
			"stun:185.18.24.50:3478",
			"stun:172.217.213.127:19305",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.210:3478",
			"stun:143.198.60.79:3478",
			"stun:66.51.128.11:3478",
			"stun:18.191.223.12:3478",
			"stun:198.211.120.59:3478",
			"stun:80.250.1.173:3478",
			"stun:194.140.246.192:3478",
			"stun:157.161.10.32:3478",
			"stun:83.64.250.246:3478",
			"stun:217.19.174.42:3478",
			"stun:77.72.169.212:3478",
			"stun:77.72.169.210:3478",
			"stun:195.208.107.138:3478",
			"stun:217.19.174.41:3478",
			"stun:77.72.169.210:3478",
			"stun:212.227.67.34:3478",
			"stun:109.235.234.65:3478",
			"stun:136.243.202.77:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.213:3478",
			"stun:52.24.174.49:3478",
			"stun:88.198.151.128:3478",
			"stun:185.45.152.22:3478",
			"stun:77.72.169.210:3478",
			"stun:173.255.213.166:3478",
			"stun:77.72.169.213:3478",
			"stun:77.72.169.210:3478",
			"stun:70.42.198.30:3478",
			"stun:77.72.169.210:3478",
			"stun:185.18.24.50:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.210:3478",
			"stun:77.72.169.210:3478",
			"stun:95.216.78.222:3478",
			"stun:83.211.9.232:3478",
			"stun:81.173.115.217:3478",
			"stun:83.125.8.47:3478",
			"stun:82.97.157.254:3478",
			"stun:217.10.68.145:3478",
			"stun:176.62.31.10:3478",
			"stun:185.39.86.17:3478",
			"stun:77.72.169.211:3478",
			"stun:77.72.169.211:3478",
			"stun:195.254.254.20:3478",
			"stun:185.112.247.26:3478",
			"stun:77.72.169.213:3478",
			"stun:52.52.70.85:3478",
			"stun:77.72.169.210:3478",
			"stun:95.217.228.176:3478",
			"stun:172.105.99.33:3478",
			"stun:34.73.197.25:3478",
			"stun:77.72.169.210:3478",
		},
	}
	broflake.StartBroflakeCensoredPeerIfNecessary(true, fopts)

	/*
		if f.FeatureEnabled(config.FeatureBroflake) {
			var fopts config.BroflakeOptions
			if err := f.FeatureOptions(config.FeatureBroflake, &fopts); err != nil {
				log.Debugf(
					"enabling broflake: bad or no options for broflake feature in global-config: %v",
					err,
				)
				return
			}
			broflake.StartBroflakeCensoredPeerIfNecessary(true, &fopts)
		}*/
}

func (f *Flashlight) applyProxiedFlow(cfg *config.Global) {
	proxied.SetProxiedFlowFeatureEnabled(true)
	//	proxied.SetProxiedFlowFeatureEnabled(f.FeatureEnabled(config.FeatureProxiedFlow))
}

// New creates a client proxy.
func New(
	appName string,
	configDir string,
	enableVPN bool,
	disconnected func() bool,
	_proxyAll func() bool,
	_googleAds func() bool,
	allowPrivateHosts func() bool,
	autoReport func() bool,
	flagsAsMap map[string]interface{},
	onConfigUpdate func(*config.Global, config.Source),
	onProxiesUpdate func([]balancer.Dialer, config.Source),
	userConfig common.UserConfig,
	statsTracker stats.Tracker,
	isPro func() bool,
	lang func() string,
	adSwapTargetURL func() string,
	reverseDNS func(host string) (string, error),
	adTrackUrl func() string,
	eventWithLabel func(category, action, label string),
	dhtupContext *dhtup.Context,
) (*Flashlight, error) {
	log.Debugf("Running in app: %v", appName)
	log.Debugf("Using configdir: %v", configDir)

	if onProxiesUpdate == nil {
		onProxiesUpdate = func(_ []balancer.Dialer, src config.Source) {}
	}
	if onConfigUpdate == nil {
		onConfigUpdate = func(_ *config.Global, src config.Source) {}
	}
	displayVersion()
	deviceID := userConfig.GetDeviceID()
	if common.InDevelopment() {
		log.Debugf("You can query for this device's activity in borda under device id: %v", deviceID)
	}
	fops.InitGlobalContext(appName, deviceID, isPro, func() string { return geolookup.GetCountry(0) })
	email.SetHTTPClient(proxied.DirectThenFrontedClient(1 * time.Minute))

	f := &Flashlight{
		configDir:         configDir,
		flagsAsMap:        flagsAsMap,
		userConfig:        userConfig,
		isPro:             isPro,
		global:            nil,
		onProxiesUpdate:   onProxiesUpdate,
		onConfigUpdate:    onConfigUpdate,
		onBordaConfigured: make(chan bool, 1),
		autoReport:        autoReport,
		op:                fops.Begin("client_started"),
		errorHandler: func(t HandledErrorType, err error) {
			log.Errorf("%v: %v", t, err)
		},
		dhtupContext:   dhtupContext,
		proxyListeners: make([]func(map[string]*commonconfig.ProxyConfig, config.Source), 0),
	}

	f.addProxyListener(func(proxies map[string]*commonconfig.ProxyConfig, src config.Source) {
		log.Debug("Applying proxy config with proxies")
		dialers := f.client.Configure(chained.CopyConfigs(proxies))
		if dialers != nil {
			f.onProxiesUpdate(dialers, src)
		}
	})

	var grabber dnsgrab.Server
	var grabberErr error
	if enableVPN {
		grabber, grabberErr = dnsgrab.Listen(50000,
			"127.0.0.1:53",
			func() string { return "8.8.8.8" })
		if grabberErr != nil {
			log.Errorf("dnsgrab unable to listen: %v", grabberErr)
		}

		go func() {
			if err := grabber.Serve(); err != nil {
				log.Errorf("dnsgrab stopped serving: %v", err)
			}
		}()

		reverseDNS = func(addr string) (string, error) {
			host, port, splitErr := net.SplitHostPort(addr)
			if splitErr != nil {
				host = addr
			}
			ip := net.ParseIP(host)
			if ip == nil {
				log.Debugf("Unable to parse IP %v, passing through address as is", host)
				return addr, nil
			}
			updatedHost, ok := grabber.ReverseLookup(ip)
			if !ok {
				// This means that the IP is one of our fake IPs (like 240.0.0.5) but dnsgrab doesn't know it. We cache dnsgrab entries
				// on disk for 24 hours, so this should almost never happen.
				return "", errors.New("Invalid IP address")
			}
			if splitErr != nil {
				return updatedHost, nil
			}
			return fmt.Sprintf("%v:%v", updatedHost, port), nil
		}
	}

	useShortcut := func() bool {
		return !_proxyAll() && f.FeatureEnabled(config.FeatureShortcut) && !f.FeatureEnabled(config.FeatureProxyWhitelistedOnly)
	}

	useDetour := func() bool {
		return !_proxyAll() && f.FeatureEnabled(config.FeatureDetour) && !f.FeatureEnabled(config.FeatureProxyWhitelistedOnly)
	}

	proxyAll := func() bool {
		useShortcutOrDetour := useShortcut() || useDetour()
		return !useShortcutOrDetour && !f.FeatureEnabled(config.FeatureProxyWhitelistedOnly)
	}

	cl, err := client.NewClient(
		f.configDir,
		disconnected,
		func() bool { return f.FeatureEnabled(config.FeatureProbeProxies) },
		proxyAll,
		useShortcut,
		shortcut.Allow,
		useDetour,
		func() bool {
			return !f.FeatureEnabled(config.FeatureNoHTTPSEverywhere)
		},
		func() bool {
			return common.Platform != "android" && (f.FeatureEnabled(config.FeatureTrackYouTube) || f.FeatureEnabled(config.FeatureGoogleSearchAds))
		},
		func() bool {
			return _googleAds() && f.FeatureEnabled(config.FeatureGoogleSearchAds)
		},
		userConfig,
		statsTracker,
		allowPrivateHosts,
		lang,
		adSwapTargetURL,
		reverseDNS,
		adTrackUrl,
		eventWithLabel,
	)
	if err != nil {
		fatalErr := fmt.Errorf("Unable to initialize client: %v", err)
		f.op.FailIf(fatalErr)
		f.op.End()
		return nil, fatalErr
	}
	f.client = cl
	return f, nil
}

// Run starts background services and runs the client proxy. It blocks as long as
// the proxy is running.
func (f *Flashlight) Run(httpProxyAddr, socksProxyAddr string,
	afterStart func(cl *client.Client),
	onError func(err error),
) {
	stop := f.StartBackgroundServices()
	defer stop()

	f.RunClientListeners(httpProxyAddr, socksProxyAddr, afterStart, onError)
}

// Starts background services like config fetching
func (f *Flashlight) StartBackgroundServices() func() {
	log.Debug("Starting client proxy background services")
	// check # of goroutines every minute, print the top 5 stacks with most
	// goroutines if the # exceeds 800 and is increasing.
	stopMonitor := goroutines.Monitor(time.Minute, 800, 5)

	stopBypass := bypass.Start(f.addProxyListener, f.configDir, f.userConfig)

	stopConfigFetch := f.startConfigFetch()
	geolookup.EnablePersistence(filepath.Join(f.configDir, "latestgeoinfo.json"))
	geolookup.Refresh()

	return func() {
		stopConfigFetch()
		stopMonitor()
		stopBypass()
	}
}

// Runs client listeners, blocking as long as the proxy is running.
func (f *Flashlight) RunClientListeners(httpProxyAddr, socksProxyAddr string,
	afterStart func(cl *client.Client),
	onError func(err error),
) {
	// Until we know our country, default to IR which has all detection rules
	log.Debug("Defaulting detour country to IR until real country is known")
	detour.SetCountry("IR")
	go func() {
		country := geolookup.GetCountry(eventual.Forever)
		log.Debugf("Setting detour country to %v", country)
		detour.SetCountry(country)
	}()

	if socksProxyAddr != "" {
		go func() {
			log.Debug("Starting client SOCKS5 proxy")
			err := f.client.ListenAndServeSOCKS5(socksProxyAddr)
			if err != nil {
				log.Errorf("Unable to start SOCKS5 proxy: %v", err)
			}
		}()
	}

	if onError == nil {
		onError = func(_ error) {}
	}
	onGeo := geolookup.OnRefresh()

	log.Debug("Starting client HTTP proxy")
	err := f.client.ListenAndServeHTTP(httpProxyAddr, func() {
		log.Debug("Started client HTTP proxy")
		proxied.SetProxyAddr(f.client.Addr)
		email.SetHTTPClient(proxied.DirectThenFrontedClient(1 * time.Minute))

		// wait for borda to be configured before proceeding
		select {
		case <-f.onBordaConfigured:
		case <-time.After(5 * time.Second):
			log.Debug("borda didn't get configured quickly, proceed anyway")
		}

		ops.Go(func() {
			// wait for geo info before reporting so that we know the client ip and
			// country
			select {
			case <-onGeo:
			case <-time.After(5 * time.Minute):
				log.Debug("failed to get geolocation info within 5 minutes, just record end of startup anyway")
			}
			f.op.End()
		})

		if afterStart != nil {
			afterStart(f.client)
		}
	})
	if err != nil {
		log.Errorf("Error running client proxy: %v", err)
		onError(err)
	}
}

// SetErrorHandler configures error handling. All errors provided to the handler are significant,
// but not enough to stop operation of the Flashlight instance. This method must be called before
// calling Run. All errors provided to the handler will be of a HandledErrorType defined in this
// package. The handler may be called multiple times concurrently.
//
// If no handler is configured, these errors will be logged on the ERROR level.
func (f *Flashlight) SetErrorHandler(handler func(t HandledErrorType, err error)) {
	if handler == nil {
		return
	}
	f.errorHandler = handler
}

// Stops the local proxy
func (f *Flashlight) Stop() error {
	return f.client.Stop()
}

func (f *Flashlight) applyClientConfig(cfg *config.Global) {
	f.client.DNSResolutionMapForDirectDialsEventual.Set(cfg.Client.DNSResolutionMapForDirectDials)
	certs, err := cfg.TrustedCACerts()
	if err != nil {
		log.Errorf("Unable to get trusted ca certs, not configuring fronted: %s", err)
	} else if cfg.Client != nil && cfg.Client.Fronted != nil {
		fronted.Configure(certs, cfg.Client.FrontedProviders(), config.DefaultFrontedProviderID, filepath.Join(f.configDir, "masquerade_cache"))
	} else {
		log.Errorf("Unable to configured fronted (no config)")
	}
}

func displayVersion() {
	log.Debugf("---- flashlight version: %s, release: %s, build revision date: %s, build date: %s ----",
		common.Version, common.PackageVersion, common.RevisionDate, common.BuildDate)
}
