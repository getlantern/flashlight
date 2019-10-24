package flashlight

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"path/filepath"
	"sync"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/fronted"
	"github.com/getlantern/golog"
	"github.com/getlantern/mtime"
	"github.com/getlantern/ops"
	"github.com/getlantern/proxybench"

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
)

var (
	log = golog.LoggerFor("flashlight")

	// FullyReportedOps are ops which are reported at 100% to borda, irrespective
	// of the borda sample percentage. This should all be low-volume operations,
	// otherwise we will utilize too much bandwidth on the client.
	FullyReportedOps = []string{"proxybench", "client_started", "client_stopped", "connect", "disconnect", "traffic", "catchall_fatal", "sysproxy_on", "sysproxy_off", "sysproxy_off_force", "sysproxy_clear", "report_issue", "proxy_rank", "proxy_selection_stability", "probe", "balancer_dial", "proxy_dial"}

	// LightweightOps are ops for which we record less than the full set of dimensions (e.g. omit error)
	LightweightOps = []string{"balancer_dial", "proxy_dial"}

	startProxyBenchOnce sync.Once
)

// Run runs a client proxy. It blocks as long as the proxy is running.
func Run(httpProxyAddr string,
	socksProxyAddr string,
	configDir string,
	disconnected func() bool,
	_useShortcut func() bool,
	_useDetour func() bool,
	allowPrivateHosts func() bool,
	autoReport func() bool,
	flagsAsMap map[string]interface{},
	beforeStart func() bool,
	afterStart func(cl *client.Client),
	onConfigUpdate func(cfg *config.Global),
	onProxiesUpdate func(map[string]*chained.ChainedServerInfo),
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
		onProxiesUpdate = func(_ map[string]*chained.ChainedServerInfo) {}
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
		cl.Configure(proxyMap)
		onProxiesUpdate(proxyMap)
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

	enableBorda := func(ctx map[string]interface{}) bool {
		if !autoReport() {
			// User has chosen not to automatically submit data
			return false
		}

		if rand.Float64() <= cfg.BordaSamplePercentage/100 {
			// Randomly included in sample
			return true
		}

		delete(ctx, "beam") // beam is only useful within a single client session.
		// For some ops, we don't randomly sample, we include all of them
		op := ctx["op"]
		switch t := op.(type) {
		case string:
			for _, lightweightOp := range LightweightOps {
				if t == lightweightOp {
					log.Tracef("Removing high dimensional data for lightweight op %v", lightweightOp)
					delete(ctx, "error")
					delete(ctx, "error_text")
					delete(ctx, "origin")
					delete(ctx, "origin_host")
					delete(ctx, "origin_port")
					delete(ctx, "root_op")
				}
			}

			for _, fullyReportedOp := range FullyReportedOps {
				if t == fullyReportedOp {
					log.Tracef("Including fully reported op %v in borda sample", fullyReportedOp)
					return true
				}
			}
			return false
		default:
			return false
		}
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
