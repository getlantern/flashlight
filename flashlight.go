package flashlight

import (
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"time"

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

	"github.com/getlantern/common/apipb"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/borda"
	"github.com/getlantern/flashlight/bypass"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/domainrouting"
	"github.com/getlantern/flashlight/email"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/goroutines"
	fops "github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/otel"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/shortcut"
	"github.com/getlantern/flashlight/stats"
)

var (
	log = golog.LoggerFor("flashlight")

	startProxyBenchOnce sync.Once

	// blockingRelevantFeatures lists all features that might affect blocking and gives their
	// default enabled status (until we know the country)
	blockingRelevantFeatures = map[string]bool{
		config.FeatureProxyBench:           false,
		config.FeatureGoogleSearchAds:      false,
		config.FeaturePingProxies:          false,
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
	OnProxies(map[string]*apipb.ProxyConfig)
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
	proxyListeners    []func(map[string]*apipb.ProxyConfig)
}

func (f *Flashlight) onGlobalConfig(cfg *config.Global, src config.Source) {
	log.Debugf("Got global config from %v", src)
	f.mxGlobal.Lock()
	f.global = cfg
	f.mxGlobal.Unlock()
	domainrouting.Configure(cfg.DomainRoutingRules, cfg.ProxiedSites)
	f.applyClientConfig(cfg)
	f.applyProxyBench(cfg)
	f.applyBorda(cfg)
	f.applyOtel(cfg)
	select {
	case f.onBordaConfigured <- true:
		// okay
	default:
		// ignore
	}
	f.onConfigUpdate(cfg, src)
	f.reconfigurePingProxies()
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

func (f *Flashlight) reconfigurePingProxies() {
	enabled := func() bool {
		return common.InDevelopment() ||
			(f.FeatureEnabled(config.FeaturePingProxies) && f.autoReport())
	}
	var opts config.PingProxiesOptions
	// ignore the error because the zero value means disabling it.
	_ = f.FeatureOptions(config.FeaturePingProxies, &opts)
	f.client.ConfigurePingProxies(enabled, opts.Interval)
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

func (f *Flashlight) AddProxyListener(listener func(map[string]*apipb.ProxyConfig)) {
	f.mxProxyListeners.Lock()
	defer f.mxProxyListeners.Unlock()
	f.proxyListeners = append(f.proxyListeners, listener)
}

func (f *Flashlight) notifyProxyListeners(proxies map[string]*apipb.ProxyConfig) {
	f.mxProxyListeners.RLock()
	defer f.mxProxyListeners.RUnlock()
	for _, l := range f.proxyListeners {
		go l(proxies)
	}
}

func (f *Flashlight) startConfigFetch() func() {
	proxiesDispatch := func(conf interface{}, src config.Source) {
		proxyMap := conf.(map[string]*apipb.ProxyConfig)
		f.notifyProxyListeners(proxyMap)
		log.Debugf("Applying proxy config with proxies: %v", proxyMap)
		dialers := f.client.Configure(proxyMap)
		if dialers != nil {
			f.onProxiesUpdate(dialers, src)
		}
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
	if cfg.Otel != nil && f.FeatureEnabled(config.FeatureOtel) {
		otel.Configure(cfg.Otel)
	}
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
		dhtupContext: dhtupContext,
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

	stopBypass := bypass.Start(f.AddProxyListener, f.configDir, f.userConfig)

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
