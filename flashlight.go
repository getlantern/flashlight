package flashlight

import (
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/detour"
	"github.com/getlantern/dnsgrab"
	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/fronted"
	"github.com/getlantern/golog"
	"github.com/getlantern/mtime"
	"github.com/getlantern/ops"
	"github.com/getlantern/proxybench"
	"github.com/getlantern/trafficlog-flashlight/tlproc"

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
		config.FeatureProxyBench:           false,
		config.FeaturePingProxies:          false,
		config.FeatureNoBorda:              true,
		config.FeatureNoProbeProxies:       true,
		config.FeatureNoDetour:             true,
		config.FeatureNoHTTPSEverywhere:    true,
		config.FeatureProxyWhitelistedOnly: true,
	}
)

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
	vpnEnabled        bool
	op                *fops.Op

	// If both the trafficLogLock and proxiesLock are needed, the trafficLogLock should be obtained
	// first. Keeping the order consistent avoids deadlocking.

	// Log of network traffic to and from the proxies. Used to attach packet capture files to
	// reported issues. Nil if traffic logging is not enabled.
	trafficLog     *tlproc.TrafficLogProcess
	trafficLogLock sync.RWMutex

	// Also protected by trafficLogLock.
	captureSaveDuration time.Duration

	// proxies are tracked by the application solely for data collection purposes. This value should
	// not be changed, except by Flashlight.onProxiesUpdate. State-changing methods on the dialers
	// should not be called. In short, this slice and its elements should be treated as read-only.
	proxies     []balancer.Dialer
	proxiesLock sync.RWMutex
}

func (f *Flashlight) onGlobalConfig(cfg *config.Global, src config.Source) {
	f.mxGlobal.Lock()
	f.global = cfg
	f.mxGlobal.Unlock()
	domainrouting.Configure(cfg.DomainRoutingRules, cfg.ProxiedSites)
	applyClientConfig(cfg)
	f.applyProxyBench(cfg)
	f.applyBorda(cfg)
	select {
	case f.onBordaConfigured <- true:
		// okay
	default:
		// ignore
	}
	f.onConfigUpdate(cfg, src)
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
	proxiesDispatch := func(conf interface{}, src config.Source) {
		proxyMap := conf.(map[string]*chained.ChainedServerInfo)
		log.Debugf("Applying proxy config with proxies: %v", proxyMap)
		dialers := f.client.Configure(proxyMap)
		f.handleProxiesUpdate(dialers, src)
	}
	globalDispatch := func(conf interface{}, src config.Source) {
		cfg := conf.(*config.Global)
		log.Debugf("Applying global config")
		f.handleConfigUpdate(cfg, src)
	}
	rt := proxied.ParallelPreferChained()

	stopConfig := config.Init(f.configDir, f.flagsAsMap, f.userConfig, proxiesDispatch, globalDispatch, rt)
	return stopConfig
}

func (f *Flashlight) handleProxiesUpdate(dialers []balancer.Dialer, src config.Source) {
	if dialers == nil {
		return
	}
	f.onProxiesUpdate(dialers, src)
	f.trafficLogLock.Lock()
	f.proxiesLock.Lock()
	f.proxies = dialers
	if f.trafficLog != nil {
		proxyAddresses := []string{}
		for _, p := range dialers {
			proxyAddresses = append(proxyAddresses, p.Addr())
		}
		if err := f.trafficLog.UpdateAddresses(proxyAddresses); err != nil {
			log.Errorf("failed to update traffic log addresses: %v", err)
		}
	}
	f.proxiesLock.Unlock()
	f.trafficLogLock.Unlock()
}

func (f *Flashlight) handleConfigUpdate(cfg *config.Global, src config.Source) {
	f.onGlobalConfig(cfg, src)
	go f.configureTrafficLog(cfg)
}

func (f *Flashlight) applyProxyBench(cfg *config.Global) {
	go func() {
		// Wait a while for geolookup to happen before checking if we can turn on proxybench
		geolookup.GetCountry(1 * time.Minute)
		if f.featureEnabled(config.FeatureProxyBench) {
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

		if f.featureEnabled(config.FeatureNoBorda) {
			// Borda is disabled by global config
			return false
		}

		return _enableBorda(ctx)
	}
	borda.Configure(cfg.BordaReportInterval, enableBorda)
}

// GetProxies returns the currently configured proxies. State-changing methods on these dialers
// should not be called. In short, the elements of this slice should be treated as read-only.
func (f *Flashlight) GetProxies() []balancer.Dialer {
	f.proxiesLock.RLock()
	copied := make([]balancer.Dialer, len(f.proxies))
	copy(copied, f.proxies)
	f.proxiesLock.RUnlock()
	return copied
}

// New creates a client proxy.
func New(
	configDir string,
	enableVPN bool,
	disconnected func() bool,
	_proxyAll func() bool,
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
	reverseDNS func(host string) string) (*Flashlight, error) {

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
	fops.InitGlobalContext(deviceID, isPro, func() string { return geolookup.GetCountry(0) })
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
		return !_proxyAll() && !f.featureEnabled(config.FeatureNoShortcut) && !f.featureEnabled(config.FeatureProxyWhitelistedOnly)
	}

	useDetour := func() bool {
		return !_proxyAll() && !f.featureEnabled(config.FeatureNoDetour) && !f.featureEnabled(config.FeatureProxyWhitelistedOnly)
	}

	proxyAll := func() bool {
		useShortcutOrDetour := useShortcut() || useDetour()
		return !useShortcutOrDetour && !f.featureEnabled(config.FeatureProxyWhitelistedOnly)
	}

	cl, err := client.NewClient(
		disconnected,
		func() bool { return !f.featureEnabled(config.FeatureNoProbeProxies) },
		proxyAll,
		useShortcut,
		shortcut.Allow,
		useDetour,
		func() bool { return !f.featureEnabled(config.FeatureNoHTTPSEverywhere) },
		func() bool { return common.Platform != "android" && f.featureEnabled(config.FeatureTrackYouTube) },
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
