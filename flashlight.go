package flashlight

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/dnsgrab"
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
)

// Run runs a client proxy. It blocks as long as the proxy is running.
func Run(httpProxyAddr string,
	socksProxyAddr string,
	configDir string,
	vpnEnabled bool,
	disconnected func() bool,
	_useShortcut func() bool,
	_useDetour func() bool,
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

	useShortcut := func() bool {
		return !common.InStealthMode() && _useShortcut()
	}

	useDetour := func() bool {
		return !common.InStealthMode() && _useDetour()
	}

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

	cl, err := client.NewClient(
		disconnected,
		func(ctx context.Context, addr string) (bool, net.IP) {
			if useShortcut() {
				return shortcut.Allow(ctx, addr)
			}
			return false, nil
		},
		useDetour,
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

	proxied.SetProxyAddr(cl.Addr)

	onBordaConfigured := make(chan bool, 1)
	proxiesDispatch := func(conf interface{}) {
		proxyMap := conf.(map[string]*chained.ChainedServerInfo)
		log.Debugf("Applying proxy config with proxies: %v", proxyMap)
		dialers := cl.Configure(proxyMap)
		if dialers != nil {
			onProxiesUpdate(dialers)
		}
	}
	globalDispatch := func(conf interface{}) {
		// Don't love the straight cast here, but we're also the ones defining
		// the type in the factory method above.
		cfg := conf.(*config.Global)
		log.Debugf("Applying global config")
		applyClientConfig(cfg, autoReport, onBordaConfigured)
		common.SetStealthMode(cfg.StealthMode)
		onConfigUpdate(cfg)
		if common.Platform != "android" && autoReport() {
			cl.PingProxies(cfg.PingSamplePercentage)
		}
	}
	rt := proxied.ParallelPreferChained()
	stopConfig := config.Init(configDir, flagsAsMap, userConfig, proxiesDispatch, globalDispatch, rt)
	defer stopConfig()

	if beforeStart() {
		log.Debug("Preparing to start client proxy")
		onGeo := geolookup.OnRefresh()
		geolookup.Refresh()

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
			case <-onBordaConfigured:
				// okay, now we're ready
			case <-time.After(5 * time.Second):
				// borda didn't get configured quickly, proceed anyway
				// anyway
			}

			ops.Go(func() {
				// wait for geo info before reporting so that we know the client ip and
				// country
				select {
				case <-onGeo:
					// good to go
				case <-time.After(5 * time.Minute):
					// failed to get geolocation info within 5 minutes, just record end of
					// startup anyway
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

func applyClientConfig(cfg *config.Global, autoReport func() bool, onBordaConfigured chan bool) {
	certs, err := cfg.TrustedCACerts()
	if err != nil {
		log.Errorf("Unable to get trusted ca certs, not configuring fronted: %s", err)
	} else if cfg.Client != nil && cfg.Client.Fronted != nil {
		fronted.Configure(certs, cfg.Client.FrontedProviders(), config.CloudfrontProviderID, filepath.Join(appdir.General("Lantern"), "masquerade_cache"))
		chained.ConfigureFronting(certs, cfg.Client.FrontedProviders(), appdir.General("Lantern"))
	} else {
		log.Errorf("Unable to configured fronted (no config)")
	}

	// proxybench is governed by this configuration as well.
	if cfg.BordaReportInterval > 0 && !cfg.StealthMode {
		startProxyBenchOnce.Do(func() {
			proxybench.Start(&proxybench.Opts{}, func(timing time.Duration, ctx map[string]interface{}) {})
		})
	}

	_enableBorda := borda.Enabler(cfg.BordaSamplePercentage)
	enableBorda := func(ctx map[string]interface{}) bool {
		if !autoReport() {
			// User has chosen not to automatically submit data
			return false
		}

		return _enableBorda(ctx)
	}

	borda.Configure(cfg.BordaReportInterval, enableBorda)

	select {
	case onBordaConfigured <- true:
		// okay
	default:
		// ignore
	}
}

func displayVersion() {
	log.Debugf("---- flashlight version: %s, release: %s, build revision date: %s, build date: %s ----",
		common.Version, common.PackageVersion, common.RevisionDate, common.BuildDate)
}
