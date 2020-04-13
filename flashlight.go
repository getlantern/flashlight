package flashlight

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/detour"
	"github.com/getlantern/dnsgrab"
	"github.com/getlantern/eventual"
	"github.com/getlantern/fronted"
	"github.com/getlantern/golog"
	"github.com/getlantern/mtime"
	"github.com/getlantern/ops"
	"github.com/getlantern/proxybench"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/borda"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/domainrouting"
	"github.com/getlantern/flashlight/email"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/goroutines"
	fops "github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/shortcut"
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/vpn"
)

var (
	log = golog.LoggerFor("flashlight")

	startProxyBenchOnce sync.Once

	// blockingRelevantFeatures lists all features that might affect blocking and gives their
	// default enabled status (until we know the country)
	blockingRelevantFeatures = map[string]bool{
		config.FeatureProxyBench:        false,
		config.FeaturePingProxies:       false,
		config.FeatureNoBorda:           true,
		config.FeatureNoProbeProxies:    true,
		config.FeatureNoDetour:          true,
		config.FeatureNoHTTPSEverywhere: true,
	}
)

type Flashlight struct {
	configDir         string
	flagsAsMap        map[string]interface{}
	userConfig        common.UserConfig
	isPro             func() bool
	mxGlobal          sync.RWMutex
	global            *config.Global
	onProxiesUpdate   func([]balancer.Dialer)
	onConfigUpdate    func(*config.Global)
	onBordaConfigured chan bool
	autoReport        func() bool
	client            *client.Client
	vpnEnabled        bool
	op                *fops.Op
}

func (f *Flashlight) onGlobalConfig(cfg *config.Global) {
	f.mxGlobal.Lock()
	f.global = cfg
	f.mxGlobal.Unlock()
	domainrouting.Configure(cfg.DomainRoutingRules, cfg.ProxiedSites)
	applyClientConfig(cfg)
	f.applyProxyBenchAndBorda(cfg)
	select {
	case f.onBordaConfigured <- true:
		// okay
	default:
		// ignore
	}
	f.onConfigUpdate(cfg)
	f.reconfigurePingProxies()
}

func (f *Flashlight) reconfigurePingProxies() {
	enabled := func() bool {
		return common.InDevelopment() ||
			(f.featureEnabled(config.FeaturePingProxies) && f.autoReport())
	}
	var opts config.PingProxiesOptions
	// ignore the error because the zero value means disabling it.
	_ = f.featureOptions(config.FeaturePingProxies, &opts)
	f.client.ConfigurePingProxies(enabled, opts.Interval)
}

// EnabledFeatures gets all features enabled based on current conditions
func (f *Flashlight) EnabledFeatures() map[string]bool {
	f.mxGlobal.RLock()
	global := f.global
	f.mxGlobal.RUnlock()
	featuresEnabled := make(map[string]bool)
	country := geolookup.GetCountry(0)
	for feature := range global.FeaturesEnabled {
		if f.calcFeature(global, country, feature) {
			featuresEnabled[feature] = true
		}
	}
	return featuresEnabled
}

func (f *Flashlight) featureEnabled(feature string) bool {
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
		f.userConfig.GetUserID(),
		f.isPro(),
		country)
}

func (f *Flashlight) featureOptions(feature string, opts config.FeatureOptions) error {
	f.mxGlobal.RLock()
	global := f.global
	f.mxGlobal.RUnlock()
	if global == nil {
		// just to be safe
		return errors.New("No global configuration")
	}
	return global.UnmarshalFeatureOptions(feature, opts)
}

func (f *Flashlight) startConfigFetch() func() {
	proxiesDispatch := func(conf interface{}) {
		proxyMap := conf.(map[string]*chained.ChainedServerInfo)
		log.Debugf("Applying proxy config with proxies: %v", proxyMap)
		dialers := f.client.Configure(proxyMap)
		if dialers != nil {
			f.onProxiesUpdate(dialers)
		}
	}
	globalDispatch := func(conf interface{}) {
		cfg := conf.(*config.Global)
		log.Debugf("Applying global config")
		f.onGlobalConfig(cfg)
	}
	rt := proxied.ParallelPreferChained()

	stopConfig := config.Init(f.configDir, f.flagsAsMap, f.userConfig, proxiesDispatch, globalDispatch, rt)
	return stopConfig
}

func (f *Flashlight) applyProxyBenchAndBorda(cfg *config.Global) {
	if f.featureEnabled(config.FeatureProxyBench) {
		startProxyBenchOnce.Do(func() {
			proxybench.Start(&proxybench.Opts{}, func(timing time.Duration, ctx map[string]interface{}) {})
		})
	}

	_enableBorda := borda.Enabler(cfg.BordaSamplePercentage)
	enableBorda := func(ctx map[string]interface{}) bool {
		if !f.autoReport() {
			// User has chosen not to automatically submit data
			return false
		}

		if f.featureEnabled(config.FeatureNoBorda) {
			// Borda is disabled by global config
			return false
		}

		return _enableBorda(ctx)
	}
	borda.Configure(cfg.BordaReportInterval, enableBorda)
}

// New creates a client proxy.
func New(
	configDir string,
	enableVPN bool,
	disconnected func() bool,
	_useShortcut func() bool,
	_useDetour func() bool,
	allowPrivateHosts func() bool,
	autoReport func() bool,
	flagsAsMap map[string]interface{},
	onConfigUpdate func(cfg *config.Global),
	onProxiesUpdate func([]balancer.Dialer),
	userConfig common.UserConfig,
	statsTracker stats.Tracker,
	isPro func() bool,
	lang func() string,
	adSwapTargetURL func() string,
	reverseDNS func(host string) string) (*Flashlight, error) {

	if onProxiesUpdate == nil {
		onProxiesUpdate = func(_ []balancer.Dialer) {}
	}
	if onConfigUpdate == nil {
		onConfigUpdate = func(_ *config.Global) {}
	}
	displayVersion()
	deviceID := userConfig.GetDeviceID()
	if common.InDevelopment() {
		log.Debugf("You can query for this device's activity in borda under device id: %v", deviceID)
	}
	fops.InitGlobalContext(deviceID, isPro, userConfig.GetUserID, func() string { return geolookup.GetCountry(0) }, func() string { return geolookup.GetIP(0) })

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
	}

	var grabber dnsgrab.Server
	var grabberErr error
	if enableVPN {
		grabber, grabberErr = dnsgrab.Listen(50000,
			"127.0.0.1:53",
			"8.8.8.8")
		if grabberErr != nil {
			log.Errorf("dnsgrab unable to listen: %v", grabberErr)
		}

		go func() {
			if err := grabber.Serve(); err != nil {
				log.Errorf("dnsgrab stopped serving: %v", err)
			}
		}()

		reverseDNS = func(addr string) string {
			host, port, splitErr := net.SplitHostPort(addr)
			if splitErr != nil {
				host = addr
			}
			ip := net.ParseIP(host)
			if ip == nil {
				log.Debugf("Unable to parse IP %v, passing through address as is", host)
				return addr
			}
			updatedHost := grabber.ReverseLookup(ip)
			if updatedHost == "" {
				log.Debugf("Unable to reverse lookup %v, passing through address as is (this shouldn't happen much)", ip)
				return addr
			}
			if splitErr != nil {
				return updatedHost
			}
			return fmt.Sprintf("%v:%v", updatedHost, port)
		}
		f.vpnEnabled = true
	}

	useShortcut := func() bool {
		return !f.featureEnabled(config.FeatureNoShortcut) && _useShortcut()
	}

	useDetour := func() bool {
		return !f.featureEnabled(config.FeatureNoDetour) && _useDetour()
	}

	cl, err := client.NewClient(
		disconnected,
		func() bool { return !f.featureEnabled(config.FeatureNoProbeProxies) },
		func(ctx context.Context, addr string) (bool, net.IP) {
			if useShortcut() {
				return shortcut.Allow(ctx, addr)
			}
			return false, nil
		},
		useDetour,
		func() bool { return !f.featureEnabled(config.FeatureNoHTTPSEverywhere) },
		userConfig,
		statsTracker,
		allowPrivateHosts,
		lang,
		adSwapTargetURL,
		reverseDNS,
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

// Run runs the client proxy. It blocks as long as the proxy is running.
func (f *Flashlight) Run(httpProxyAddr, socksProxyAddr string,
	afterStart func(cl *client.Client),
	onError func(err error),
) {
	if onError == nil {
		onError = func(_ error) {}
	}

	// check # of goroutines every minute, print the top 5 stacks with most
	// goroutines if the # exceeds 800 and is increasing.
	stopMonitor := goroutines.Monitor(time.Minute, 800, 5)
	defer stopMonitor()
	elapsed := mtime.Stopwatch()

	log.Debug("Preparing to start client proxy")
	stop := f.startConfigFetch()
	defer stop()
	onGeo := geolookup.OnRefresh()
	geolookup.Refresh()

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

		if f.vpnEnabled {
			log.Debug("Enabling VPN mode")
			closeVPN, vpnErr := vpn.Enable(socksProxyAddr, "192.168.1.1", "", "10.0.0.2", "255.255.255.0")
			if vpnErr != nil {
				log.Error(vpnErr)
			} else {
				defer closeVPN()
			}
		}
	}

	log.Debug("Starting client HTTP proxy")
	err := f.client.ListenAndServeHTTP(httpProxyAddr, func() {
		log.Debug("Started client HTTP proxy")
		proxied.SetProxyAddr(f.client.Addr)
		email.SetHTTPClient(proxied.DirectThenFrontedClient(1 * time.Minute))
		f.op.SetMetricSum("startup_time", float64(elapsed().Seconds()))

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

		afterStart(f.client)
	})
	if err != nil {
		log.Errorf("Error running client proxy: %v", err)
		onError(err)
	}
}

func applyClientConfig(cfg *config.Global) {
	certs, err := cfg.TrustedCACerts()
	if err != nil {
		log.Errorf("Unable to get trusted ca certs, not configuring fronted: %s", err)
	} else if cfg.Client != nil && cfg.Client.Fronted != nil {
		fronted.Configure(certs, cfg.Client.FrontedProviders(), config.CloudfrontProviderID, filepath.Join(appdir.General("Lantern"), "masquerade_cache"))
		chained.ConfigureFronting(certs, cfg.Client.FrontedProviders(), appdir.General("Lantern"))
	} else {
		log.Errorf("Unable to configured fronted (no config)")
	}
}

func displayVersion() {
	log.Debugf("---- flashlight version: %s, release: %s, build revision date: %s, build date: %s ----",
		common.Version, common.PackageVersion, common.RevisionDate, common.BuildDate)
}
