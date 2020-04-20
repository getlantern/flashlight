package flashlight

import (
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
		config.FeatureProxyBench:           false,
		config.FeaturePingProxies:          false,
		config.FeatureNoBorda:              true,
		config.FeatureNoProbeProxies:       true,
		config.FeatureNoDetour:             true,
		config.FeatureNoHTTPSEverywhere:    true,
		config.FeatureProxyWhitelistedOnly: true,
	}
)

type runner struct {
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
}

func (r *runner) onGlobalConfig(cfg *config.Global) {
	r.mxGlobal.Lock()
	r.global = cfg
	r.mxGlobal.Unlock()
	domainrouting.Configure(cfg.DomainRoutingRules, cfg.ProxiedSites)
	applyClientConfig(cfg)
	r.applyProxyBenchAndBorda(cfg)
	select {
	case r.onBordaConfigured <- true:
		// okay
	default:
		// ignore
	}
	r.onConfigUpdate(cfg)
	r.reconfigurePingProxies()
}

func (r *runner) reconfigurePingProxies() {
	enabled := func() bool {
		return common.InDevelopment() ||
			(r.featureEnabled(config.FeaturePingProxies) && r.autoReport())
	}
	var opts config.PingProxiesOptions
	// ignore the error because the zero value means disabling it.
	_ = r.featureOptions(config.FeaturePingProxies, &opts)
	r.client.ConfigurePingProxies(enabled, opts.Interval)
}

func (r *runner) featureEnabled(feature string) bool {
	r.mxGlobal.RLock()
	global := r.global
	r.mxGlobal.RUnlock()
	country := geolookup.GetCountry(0)

	// Sepcial case: Use defaults for blocking related features until geolookup is finished
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
		r.userConfig.GetUserID(),
		r.isPro(),
		country)
}

func (r *runner) featureOptions(feature string, opts config.FeatureOptions) error {
	r.mxGlobal.RLock()
	global := r.global
	r.mxGlobal.RUnlock()
	if global == nil {
		// just to be safe
		return errors.New("No global configuration")
	}
	return global.UnmarshalFeatureOptions(feature, opts)
}

func (r *runner) startConfigFetch() func() {
	proxiesDispatch := func(conf interface{}) {
		proxyMap := conf.(map[string]*chained.ChainedServerInfo)
		log.Debugf("Applying proxy config with proxies: %v", proxyMap)
		dialers := r.client.Configure(proxyMap)
		if dialers != nil {
			r.onProxiesUpdate(dialers)
		}
	}
	globalDispatch := func(conf interface{}) {
		cfg := conf.(*config.Global)
		log.Debugf("Applying global config")
		r.onGlobalConfig(cfg)
	}
	rt := proxied.ParallelPreferChained()

	stopConfig := config.Init(r.configDir, r.flagsAsMap, r.userConfig, proxiesDispatch, globalDispatch, rt)
	return stopConfig
}

func (r *runner) applyProxyBenchAndBorda(cfg *config.Global) {
	if r.featureEnabled(config.FeatureProxyBench) {
		startProxyBenchOnce.Do(func() {
			proxybench.Start(&proxybench.Opts{}, func(timing time.Duration, ctx map[string]interface{}) {})
		})
	}

	_enableBorda := borda.Enabler(cfg.BordaSamplePercentage)
	enableBorda := func(ctx map[string]interface{}) bool {
		if !r.autoReport() {
			// User has chosen not to automatically submit data
			return false
		}

		if r.featureEnabled(config.FeatureNoBorda) {
			// Borda is disabled by global config
			return false
		}

		return _enableBorda(ctx)
	}
	borda.Configure(cfg.BordaReportInterval, enableBorda)
}

// Run runs a client proxy. It blocks as long as the proxy is running.
func Run(httpProxyAddr string,
	socksProxyAddr string,
	configDir string,
	vpnEnabled bool,
	disconnected func() bool,
	_proxyAll func() bool,
	allowPrivateHosts func() bool,
	autoReport func() bool,
	flagsAsMap map[string]interface{},
	beforeStart func() bool,
	afterStart func(cl *client.Client),
	onConfigUpdate func(cfg *config.Global),
	onProxiesUpdate func([]balancer.Dialer),
	userConfig common.UserConfig,
	statsTracker stats.Tracker,
	onError func(err error),
	isPro func() bool,
	userID func() int64,
	lang func() string,
	adSwapTargetURL func() string,
	reverseDNS func(host string) string) error {

	if onProxiesUpdate == nil {
		onProxiesUpdate = func(_ []balancer.Dialer) {}
	}
	if onConfigUpdate == nil {
		onConfigUpdate = func(_ *config.Global) {}
	}
	if onError == nil {
		onError = func(_ error) {}
	}

	// check # of goroutines every minute, print the top 5 stacks with most
	// goroutines if the # exceeds 800 and is increasing.
	stopMonitor := goroutines.Monitor(time.Minute, 800, 5)
	defer stopMonitor()
	elapsed := mtime.Stopwatch()
	displayVersion()
	deviceID := userConfig.GetDeviceID()
	if common.InDevelopment() {
		log.Debugf("You can query for this device's activity in borda under device id: %v", deviceID)
	}
	fops.InitGlobalContext(deviceID, isPro, userID, func() string { return geolookup.GetCountry(0) }, func() string { return geolookup.GetIP(0) })
	email.SetHTTPClient(proxied.DirectThenFrontedClient(1 * time.Minute))
	op := fops.Begin("client_started")

	var grabber dnsgrab.Server
	var grabberErr error
	if vpnEnabled {
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
	}

	r := runner{configDir: configDir,
		flagsAsMap:        flagsAsMap,
		userConfig:        userConfig,
		isPro:             isPro,
		global:            nil,
		onProxiesUpdate:   onProxiesUpdate,
		onConfigUpdate:    onConfigUpdate,
		onBordaConfigured: make(chan bool, 1),
		autoReport:        autoReport,
	}

	useShortcut := func() bool {
		return !_proxyAll() && !r.featureEnabled(config.FeatureNoShortcut) && !r.featureEnabled(config.FeatureProxyWhitelistedOnly)
	}

	useDetour := func() bool {
		return !_proxyAll() && !r.featureEnabled(config.FeatureNoDetour) && !r.featureEnabled(config.FeatureProxyWhitelistedOnly)
	}

	proxyAll := func() bool {
		useShortcutOrDetour := useShortcut() || useDetour()
		return !useShortcutOrDetour && !r.featureEnabled(config.FeatureProxyWhitelistedOnly)
	}

	cl, err := client.NewClient(
		disconnected,
		func() bool { return !r.featureEnabled(config.FeatureNoProbeProxies) },
		proxyAll,
		useShortcut,
		shortcut.Allow,
		useDetour,
		func() bool { return !r.featureEnabled(config.FeatureNoHTTPSEverywhere) },
		userConfig,
		statsTracker,
		allowPrivateHosts,
		lang,
		adSwapTargetURL,
		reverseDNS,
	)
	if err != nil {
		fatalErr := fmt.Errorf("Unable to initialize client: %v", err)
		op.FailIf(fatalErr)
		op.End()
	}
	r.client = cl

	proxied.SetProxyAddr(cl.Addr)
	stop := r.startConfigFetch()
	defer stop()

	if beforeStart() {
		log.Debug("Preparing to start client proxy")
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
				err := cl.ListenAndServeSOCKS5(socksProxyAddr)
				if err != nil {
					log.Errorf("Unable to start SOCKS5 proxy: %v", err)
				}
			}()

			if vpnEnabled && grabberErr == nil {
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
		err := cl.ListenAndServeHTTP(httpProxyAddr, func() {
			log.Debug("Started client HTTP proxy")
			op.SetMetricSum("startup_time", float64(elapsed().Seconds()))

			// wait for borda to be configured before proceeding
			select {
			case <-r.onBordaConfigured:
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
				op.End()
			})

			afterStart(cl)
		})
		if err != nil {
			log.Errorf("Error running client proxy: %v", err)
			onError(err)
		}
	}

	return nil
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
