package flashlight

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/getlantern/netx"

	commonconfig "github.com/getlantern/common/config"
	"github.com/getlantern/detour"
	"github.com/getlantern/dnsgrab"
	"github.com/getlantern/errors"
	"github.com/getlantern/eventual/v2"
	"github.com/getlantern/golog"
	"google.golang.org/protobuf/proto"

	"github.com/getlantern/flashlight/v7/apipb"
	"github.com/getlantern/flashlight/v7/chained"
	"github.com/getlantern/flashlight/v7/client"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/config"
	userconfig "github.com/getlantern/flashlight/v7/config/user"
	"github.com/getlantern/flashlight/v7/dialer"
	"github.com/getlantern/flashlight/v7/domainrouting"
	"github.com/getlantern/flashlight/v7/geolookup"
	"github.com/getlantern/flashlight/v7/goroutines"
	fops "github.com/getlantern/flashlight/v7/ops"
	"github.com/getlantern/flashlight/v7/otel"
	"github.com/getlantern/flashlight/v7/sentry"
	"github.com/getlantern/flashlight/v7/services"
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

type Flashlight struct {
	configDir        string
	flagsAsMap       map[string]interface{}
	userConfig       common.UserConfig
	isPro            func() bool
	mxGlobal         sync.RWMutex
	global           *config.Global
	autoReport       func() bool
	client           *client.Client
	callbacks        clientCallbacks
	op               *fops.Op
	errorHandler     func(HandledErrorType, error)
	mxProxyListeners sync.RWMutex
	proxyListeners   []func(map[string]*commonconfig.ProxyConfig, config.Source)
}

// clientCallbacks are callbacks the client is configured with
type clientCallbacks struct {
	onInit            func()
	onProxiesUpdate   func([]dialer.ProxyDialer, config.Source)
	onConfigUpdate    func(*config.Global, config.Source)
	onDialError       func(error, bool)
	onSucceedingProxy func()
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
	allowPrivateHosts func() bool,
	autoReport func() bool,
	flagsAsMap map[string]interface{},
	userConfig common.UserConfig,
	statsTracker stats.Tracker,
	isPro func() bool,
	lang func() string,
	reverseDNS func(host string) (string, error),
	eventWithLabel func(category, action, label string),
	options ...Option,
) (*Flashlight, error) {
	if common.Platform != "ios" {
		netx.EnableNAT64AutoDiscovery()
	}
	log.Debugf("Running in app: %v", appName)
	log.Debugf("Using configdir: %v", configDir)
	displayVersion(appVersion, revisionDate)
	common.InitVersion(appVersion)
	sentry.InitSentry(appVersion)
	deviceID := userConfig.GetDeviceID()
	log.Debugf("You can query for this device's activity under device id: %v", deviceID)
	fops.InitGlobalContext(
		appName, appVersion, revisionDate, deviceID, isPro, func() string {
			return geolookup.GetCountry(0)
		})

	f := &Flashlight{
		callbacks: clientCallbacks{
			onConfigUpdate: func(*config.Global, config.Source) {
				log.Debug("[Startup] client config updated")
			},
			onInit: func() {
				log.Debug("[Startup] onInit called")
			},
			onProxiesUpdate: func(_ []dialer.ProxyDialer, src config.Source) {
				log.Debugf("[Startup] onProxiesUpdate called from %v", src)
			},
			onDialError: func(err error, hasSucceeding bool) {

			},
			onSucceedingProxy: func() {
				log.Debug("[Startup] onSucceedingProxy called")
			},
		},
		configDir:  configDir,
		flagsAsMap: flagsAsMap,
		userConfig: userConfig,
		isPro:      isPro,
		global:     nil,
		autoReport: autoReport,
		op:         fops.Begin("client_started"),
		errorHandler: func(t HandledErrorType, err error) {
			log.Errorf("%v: %v", t, err)
		},
		proxyListeners: make([]func(map[string]*commonconfig.ProxyConfig, config.Source), 0),
	}

	readable, _ := f.flagsAsMap["readableconfig"].(bool)
	sticky, _ := f.flagsAsMap["stickyconfig"].(bool)
	_, err := userconfig.Init(f.configDir, readable || sticky)
	if err != nil {
		log.Errorf("user config: %v", err)
	}

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

	for _, option := range options {
		option(f)
	}

	cl, err := client.NewClient(
		f.configDir,
		disconnected,
		proxyAll,
		useShortcut,
		shortcut.Allow,
		useDetour,
		func() bool {
			return !f.featureEnabled(config.FeatureNoHTTPSEverywhere)
		},
		userConfig,
		statsTracker,
		allowPrivateHosts,
		lang,
		reverseDNS,
		eventWithLabel,
		f.callbacks.onDialError,
		f.callbacks.onSucceedingProxy,
	)

	if err != nil {
		fatalErr := fmt.Errorf("unable to initialize client: %v", err)
		f.op.FailIf(fatalErr)
		f.op.End()
		return nil, fatalErr
	}

	f.client = cl
	globalConfig, err := config.NewGlobalOnDisk(f.configDir, f.flagsAsMap)
	if err != nil {
		fatalErr := fmt.Errorf("unable to initialize global config: %v", err)
		f.op.FailIf(fatalErr)
		f.op.End()
		return nil, fatalErr
	} else {
		f.onGlobalConfig(globalConfig, config.Embedded)
	}

	f.addProxyListener(func(proxies map[string]*commonconfig.ProxyConfig, src config.Source) {
		log.Debug("Applying proxy config with proxies")
		dialers := f.client.Configure(chained.CopyConfigs(proxies))
		log.Debugf("Got %v dialers", len(dialers))
		if dialers != nil {
			f.callbacks.onProxiesUpdate(dialers, src)
		}
	})

	fn := func(old, new *userconfig.UserConfig) {
		var country string
		if old != nil {
			country = old.GetCountry()
		}

		// update the country if it has changed
		if nc := new.GetCountry(); nc != country && nc != "" {
			log.Debugf("Setting detour country to %v", nc)
			detour.SetCountry(nc)
		}

		pconfig := new.GetProxy()
		if pconfig == nil || len(pconfig.GetProxies()) == 0 {
			return // return early since there are no new proxy configs
		}

		log.Debug("Received new proxy configs")
		proxyMap := f.convertNewProxyConfToOld(pconfig.GetProxies())
		f.notifyProxyListeners(proxyMap, config.Fetched)
	}

	// there might have been an existing config that was loaded before we start listening so we need
	// to check for that and call the listener if there was
	conf, _ := userconfig.GetConfig(eventual.DontWait)
	if conf != nil {
		fn(nil, conf)
	}

	userconfig.OnConfigChange(fn)

	return f, nil
}

// Run starts background services and runs the client proxy. It blocks as long as
// the proxy is running.
func (f *Flashlight) Run(httpProxyAddr, socksProxyAddr string,
	afterStart func(cl *client.Client),
	onError func(err error),
) {
	if country := geolookup.GetCountry(0); country == "" {
		// Until we know our country, default to IR which has all detection rules
		log.Debug("Defaulting detour country to IR until real country is known")
		detour.SetCountry("IR")
	}

	stop, err := f.StartBackgroundServices()
	if err != nil {
		log.Error(err)
	}
	defer stop()

	f.RunClientListeners(httpProxyAddr, socksProxyAddr, afterStart, onError)
}

// StartBackgroundServices starts the goroutine monitoring, bypass, and config fetching background
// services and returns a function that can be called to stop them.
func (f *Flashlight) StartBackgroundServices() (func(), error) {
	log.Debug("Starting background services")
	// check # of goroutines every minute, print the top 5 stacks with most
	// goroutines if the # exceeds 800 and is increasing.
	stopMonitor := goroutines.Monitor(time.Minute, 800, 5)
	stopGlobalConfigFetch := f.startGlobalConfigFetch()

	stopBypass := services.StartBypassService(f.addProxyListener, f.configDir, f.userConfig)

	// we don't need to start the config service if sticky is set
	if sticky, _ := f.flagsAsMap["stickyconfig"].(bool); sticky {
		log.Debug("Sticky config set, not starting config service")
		return func() {
			stopMonitor()
			stopBypass()
			stopGlobalConfigFetch()
		}, nil
	}
	stopConfigService, err := f.startConfigService()
	if err != nil {
		return func() {
			stopMonitor()
			stopBypass()
			stopGlobalConfigFetch()
		}, fmt.Errorf("unable to start config service: %w", err)
	}

	return func() {
		stopMonitor()
		stopBypass()
		stopGlobalConfigFetch()
		stopConfigService()
	}, nil
}

func (f *Flashlight) startConfigService() (services.StopFn, error) {
	readable, _ := f.flagsAsMap["readableconfig"].(bool)
	// we don't need to also check for sticky here because this function is only called if sticky is not set
	handler, err := userconfig.Init(f.configDir, readable)
	if err != nil {
		return nil, err
	}

	var url string
	if cloudURL, _ := f.flagsAsMap["cloudconfig"].(string); cloudURL != "" {
		url = cloudURL
	} else {
		url = common.UserConfigURL
	}

	configOpts := &services.ConfigOptions{
		OriginURL:  url,
		UserConfig: f.userConfig,
	}
	return services.StartConfigService(handler, configOpts)
}

func (f *Flashlight) onGlobalConfig(cfg *config.Global, src config.Source) {
	log.Debugf("Got global config from %v", src)
	f.mxGlobal.Lock()
	f.global = cfg
	f.mxGlobal.Unlock()
	domainrouting.Configure(cfg.DomainRoutingRules, cfg.ProxiedSites)
	f.applyGlobalConfig(cfg)
	f.applyOtel(cfg)
	f.callbacks.onConfigUpdate(cfg, src)
	f.callbacks.onInit()
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
		if f.calcFeature(global, country, "0.0.1", feature) {
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
		log.Debugf("Blocking related feature %v %v because geolookup has not yet finished", feature, enabledText)
		return enabled
	}
	if global == nil {
		log.Error("No global configuration!")
		return enabled
	}
	if blockingRelated {
		log.Debugf("Checking blocking related feature %v with country set to %v", feature, country)
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

func (f *Flashlight) startGlobalConfigFetch() func() {
	globalDispatch := func(conf interface{}, src config.Source) {
		cfg := conf.(*config.Global)
		log.Debugf("Applying global config")
		f.onGlobalConfig(cfg, src)
	}

	onConfigSaveError := func(err error) {
		f.errorHandler(ErrorTypeConfigSaveFailure, err)
	}

	stopConfig := config.Init(
		f.configDir, f.flagsAsMap, f.userConfig,
		globalDispatch, onConfigSaveError)
	return stopConfig
}

func (f *Flashlight) applyOtel(cfg *config.Global) {
	if cfg.Otel != nil && f.featureEnabled(config.FeatureOtel) {
		otel.Configure(cfg.Otel)
	}
}

// convertNewProxyConfToOld converts the new ProxyConnectConfig format to the old ProxyConfig. This
// is temporary until all code is updated to use the new config format.
//
// TODO: Update all code to use the new config format and remove this method.
func (f *Flashlight) convertNewProxyConfToOld(proxies []*apipb.ProxyConnectConfig) map[string]*commonconfig.ProxyConfig {
	proxyMap := make(map[string]*commonconfig.ProxyConfig)
	for _, p := range proxies {
		// use lantern-cloud to map to legacy so we don't have to write it ourselves
		lc, err := apipb.ProxyToLegacyConfig(p)
		if err != nil {
			log.Errorf("Unable to convert %s proxy config to legacy: %v", p.Name, err)
			continue
		}

		// since lantern-cloud defines legacy proxy Location as LegacyConnectConfig_ProxyLocation
		// and commonconfig defines it as ProxyConfig_Location, we can't just cast it, but we can
		// marshal/unmarshal it to get the same protobuf. This is a temporary workaround until all
		// code is updated to use the new config format. And since we know that the protobuf is the
		// same, we can ignore the errors from the marshal/unmarshal
		cc := &commonconfig.ProxyConfig{}
		lcb, _ := proto.Marshal(lc)
		_ = proto.Unmarshal(lcb, cc)

		proxyMap[p.Name] = cc
	}

	return proxyMap
}

// Runs client listeners, blocking as long as the proxy is running.
func (f *Flashlight) RunClientListeners(httpProxyAddr, socksProxyAddr string,
	afterStart func(cl *client.Client),
	onError func(err error),
) {
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

	log.Debug("Starting client HTTP proxy")
	err := f.client.ListenAndServeHTTP(httpProxyAddr, func() {
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

func (f *Flashlight) applyGlobalConfig(cfg *config.Global) {
	f.client.DNSResolutionMapForDirectDialsEventual.Set(cfg.Client.DNSResolutionMapForDirectDials)
}

func displayVersion(appVersion, revisionDate string) {
	log.Debugf("---- application version: %s, library version: %s, build revision date: %s ----",
		appVersion, common.LibraryVersion, revisionDate)
}
