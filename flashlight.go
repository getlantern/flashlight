package flashlight

import (
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"time"

	commonconfig "github.com/getlantern/common/config"
	"github.com/getlantern/detour"
	"github.com/getlantern/dnsgrab"
	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/fronted"
	"github.com/getlantern/golog"
	"github.com/getlantern/netx"
	"github.com/getlantern/ops"

	"github.com/getlantern/flashlight/v7/balancer"
	"github.com/getlantern/flashlight/v7/bypass"
	"github.com/getlantern/flashlight/v7/chained"
	"github.com/getlantern/flashlight/v7/client"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/config"
	"github.com/getlantern/flashlight/v7/domainrouting"
	"github.com/getlantern/flashlight/v7/email"
	"github.com/getlantern/flashlight/v7/geolookup"
	"github.com/getlantern/flashlight/v7/goroutines"
	fops "github.com/getlantern/flashlight/v7/ops"
	"github.com/getlantern/flashlight/v7/otel"
	"github.com/getlantern/flashlight/v7/proxied"
	"github.com/getlantern/flashlight/v7/shortcut"
	"github.com/getlantern/flashlight/v7/stats"
)

var (
	log = golog.LoggerFor("flashlight")

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
	configDir        string
	flagsAsMap       map[string]interface{}
	userConfig       common.UserConfig
	isPro            func() bool
	mxGlobal         sync.RWMutex
	global           *config.Global
	onProxiesUpdate  func([]balancer.Dialer, config.Source)
	onConfigUpdate   func(*config.Global, config.Source)
	autoReport       func() bool
	client           *client.Client
	op               *fops.Op
	errorHandler     func(HandledErrorType, error)
	mxProxyListeners sync.RWMutex
	proxyListeners   []func(map[string]*commonconfig.ProxyConfig, config.Source)
}

func (f *Flashlight) onGlobalConfig(cfg *config.Global, src config.Source) {
	log.Debugf("Got global config from %v: %#v", src, cfg)
	f.mxGlobal.Lock()
	f.global = cfg
	f.mxGlobal.Unlock()
	domainrouting.Configure(cfg.DomainRoutingRules, cfg.ProxiedSites)
	f.applyClientConfig(cfg)
	f.applyOtel(cfg)
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
	log.Debug("EnabledFeatures::Getting enabled features")
	featuresEnabled := make(map[string]bool)
	f.mxGlobal.RLock()
	if f.global == nil {
		f.mxGlobal.RUnlock()
		return featuresEnabled
	}
	global := f.global
	f.mxGlobal.RUnlock()
	country := geolookup.GetCountry(0)
	log.Debugf("EnabledFeatures::Country is %v", country)
	for feature := range global.FeaturesEnabled {
		log.Debugf("EnabledFeatures::calculating feature %v", feature)
		enabled := f.calcFeature(global, country, "0.0.1", feature)
		if enabled {
			featuresEnabled[feature] = true
		}
		log.Debugf("EnabledFeatures::feature %v is %v", feature, enabled)
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

// featureEnabled returns true if the input feature is enabled for this flashlight instance. Feature
// names are tracked in the config package.
func (f *Flashlight) featureEnabled(feature string) bool {
	// features internal to flashlight are not controllable by application version, since flashlight doesn't know the version, so we use a very low version number just to make sure it parses
	return f.FeatureEnabled(feature, "0.0.1")
}

func (f *Flashlight) FeatureEnabled(feature, applicationVersion string) bool {
	f.mxGlobal.RLock()
	global := f.global
	f.mxGlobal.RUnlock()
	return f.calcFeature(global, geolookup.GetCountry(0), applicationVersion, feature)
}

func (f *Flashlight) calcFeature(global *config.Global, country, applicationVersion, feature string) bool {
	// Special case: Use defaults for blocking related features until geolookup is finished
	// to avoid accidentally generating traffic that could trigger blocking.
	enabled, blockingRelated := blockingRelevantFeatures[feature]
	if country == "" && blockingRelated {
		enabledText := "disabled"
		if enabled {
			enabledText = "enabled"
		}
		log.Debugf("Features::Blocking related feature %v %v because geolookup has not yet finished", feature, enabledText)
		return enabled
	}
	if global == nil {
		log.Error("No global configuration!")
		return enabled
	}
	if blockingRelated {
		log.Debugf("Features::Checking blocking related feature %v with country set to %v", feature, country)
	}
	return global.FeatureEnabled(feature,
		common.Platform,
		f.userConfig.GetAppName(),
		applicationVersion,
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
	rt := proxied.ParallelPreferChained()

	onProxiesSaveError := func(err error) {
		f.errorHandler(ErrorTypeProxySaveFailure, err)
	}
	onConfigSaveError := func(err error) {
		f.errorHandler(ErrorTypeConfigSaveFailure, err)
	}

	stopConfig := config.Init(
		f.configDir, f.flagsAsMap, f.userConfig,
		proxiesDispatch, onProxiesSaveError,
		globalDispatch, onConfigSaveError, rt)
	return stopConfig
}

func (f *Flashlight) applyOtel(cfg *config.Global) {
	if cfg.Otel != nil && f.featureEnabled(config.FeatureOtel) {
		otel.Configure(cfg.Otel)
	}
}

// New creates a client proxy.
func New(
	appName string,
	appVersion string,
	revisionDate string,
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
) (*Flashlight, error) {
	log.Debugf("Running in app: %v", appName)
	log.Debugf("Using configdir: %v", configDir)

	if onProxiesUpdate == nil {
		onProxiesUpdate = func(_ []balancer.Dialer, src config.Source) {}
	}
	if onConfigUpdate == nil {
		onConfigUpdate = func(_ *config.Global, src config.Source) {}
	}
	displayVersion(appVersion, revisionDate)
	common.CompileTimeApplicationVersion = appVersion
	deviceID := userConfig.GetDeviceID()
	log.Debugf("You can query for this device's activity under device id: %v", deviceID)
	fops.InitGlobalContext(appName, appVersion, revisionDate, deviceID, isPro, func() string { return geolookup.GetCountry(0) })
	email.SetHTTPClient(proxied.DirectThenFrontedClient(1 * time.Minute))

	f := &Flashlight{
		configDir:       configDir,
		flagsAsMap:      flagsAsMap,
		userConfig:      userConfig,
		isPro:           isPro,
		global:          nil,
		onProxiesUpdate: onProxiesUpdate,
		onConfigUpdate:  onConfigUpdate,
		autoReport:      autoReport,
		op:              fops.Begin("client_started"),
		errorHandler: func(t HandledErrorType, err error) {
			log.Errorf("%v: %v", t, err)
		},
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
		return !_proxyAll() && f.featureEnabled(config.FeatureShortcut) && !f.featureEnabled(config.FeatureProxyWhitelistedOnly)
	}

	useDetour := func() bool {
		return !_proxyAll() && f.featureEnabled(config.FeatureDetour) && !f.featureEnabled(config.FeatureProxyWhitelistedOnly)
	}

	proxyAll := func() bool {
		useShortcutOrDetour := useShortcut() || useDetour()
		return !useShortcutOrDetour && !f.featureEnabled(config.FeatureProxyWhitelistedOnly)
	}

	cl, err := client.NewClient(
		f.configDir,
		disconnected,
		func() bool {
			log.Debugf("Features::checking if probe proxies are enabled")
			return f.featureEnabled(config.FeatureProbeProxies)
		},
		proxyAll,
		useShortcut,
		shortcut.Allow,
		useDetour,
		func() bool {
			return !f.featureEnabled(config.FeatureNoHTTPSEverywhere)
		},
		func() bool {
			return common.Platform != "android" && (f.featureEnabled(config.FeatureTrackYouTube) || f.featureEnabled(config.FeatureGoogleSearchAds))
		},
		func() bool {
			return _googleAds() && f.featureEnabled(config.FeatureGoogleSearchAds)
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

func displayVersion(appVersion, revisionDate string) {
	log.Debugf("---- application version: %s, library version: %s, build revision date: %s ----",
		appVersion, common.LibraryVersion, revisionDate)
}
