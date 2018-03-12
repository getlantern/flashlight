package flashlight

import (
	"crypto/x509"
	"fmt"
	"math/rand"
	"net"
	"path/filepath"
	"runtime"
	"time"

	"github.com/getlantern/appdir"
	fops "github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/shortcut"
	"github.com/getlantern/fronted"
	"github.com/getlantern/golog"
	"github.com/getlantern/jibber_jabber"
	"github.com/getlantern/keyman"
	"github.com/getlantern/mtime"
	"github.com/getlantern/ops"
	"github.com/getlantern/osversion"

	"github.com/getlantern/flashlight/borda"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/goroutines"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/stats"

	// Make sure logging is initialized
	_ "github.com/getlantern/flashlight/logging"
)

var (
	log = golog.LoggerFor("flashlight")

	// FullyReportedOps are ops which are reported at 100% to borda, irrespective
	// of the borda sample percentage. This should all be low-volume operations,
	// otherwise we will utilize too much bandwidth on the client.
	FullyReportedOps = []string{"client_started", "client_stopped", "connect", "disconnect", "traffic", "catchall_fatal", "sysproxy_on", "sysproxy_off", "sysproxy_off_force", "sysproxy_clear", "report_issue", "proxy_rank", "probe", "balancer_dial", "proxy_dial"}

	// Lightweight Ops are ops for which we record less than the full set of dimensions (e.g. omit error)
	LightweightOps = []string{"balancer_dial", "proxy_dial"}
)

// Run runs a client proxy. It blocks as long as the proxy is running.
func Run(httpProxyAddr string,
	socksProxyAddr string,
	configDir string,
	disconnected func() bool,
	useShortcut func() bool,
	useDetour func() bool,
	allowPrivateHosts func() bool,
	autoReport func() bool,
	flagsAsMap map[string]interface{},
	beforeStart func() bool,
	afterStart func(cl *client.Client),
	onConfigUpdate func(cfg *config.Global),
	userConfig common.AuthConfig,
	statsTracker stats.Tracker,
	onError func(err error),
	deviceID string,
	isPro func() bool,
	lang func() string,
	adSwapTargetURL func() string,
	adBlockingAllowed func() bool,
	reverseDNS func(host string) string) error {

	// check # of goroutines every minute, print the top 5 stacks with most
	// goroutines if the # exceeds 2000 and is increasing.
	stopMonitor := goroutines.Monitor(time.Minute, 2000, 5)
	defer stopMonitor()
	elapsed := mtime.Stopwatch()
	displayVersion()
	initContext(deviceID, common.Version, common.RevisionDate, isPro)
	op := fops.Begin("client_started")

	cl, err := client.NewClient(
		disconnected,
		func(addr string) (bool, net.IP) {
			if useShortcut() {
				return shortcut.Allow(addr)
			}
			return false, nil
		},
		useDetour,
		userConfig.GetToken,
		statsTracker,
		allowPrivateHosts,
		lang,
		adSwapTargetURL,
		adBlockingAllowed,
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
		cl.Configure(proxyMap, deviceID)
	}
	globalDispatch := func(conf interface{}) {
		// Don't love the straight cast here, but we're also the ones defining
		// the type in the factory method above.
		cfg := conf.(*config.Global)
		log.Debugf("Applying global config")
		applyClientConfig(cl, cfg, deviceID, autoReport, onBordaConfigured)
		onConfigUpdate(cfg)
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

func applyClientConfig(client *client.Client, cfg *config.Global, deviceID string, autoReport func() bool, onBordaConfigured chan bool) {
	certs, err := getTrustedCACerts(cfg)
	if err != nil {
		log.Errorf("Unable to get trusted ca certs, not configuring fronted: %s", err)
	} else if cfg.Client != nil {
		fronted.Configure(certs, cfg.Client.MasqueradeSets, filepath.Join(appdir.General("Lantern"), "masquerade_cache"))
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

		// For some ops, we don't randomly sample, we include all of them
		op := ctx["op"]
		switch t := op.(type) {
		case string:
			for _, lightweightOp := range LightweightOps {
				if t == lightweightOp {
					log.Tracef("Removing error and error_text for op %v", lightweightOp)
					delete(ctx, "error")
					delete(ctx, "error_text")
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

func getTrustedCACerts(cfg *config.Global) (pool *x509.CertPool, err error) {
	certs := make([]string, 0, len(cfg.TrustedCAs))
	for _, ca := range cfg.TrustedCAs {
		certs = append(certs, ca.Cert)
	}
	pool, err = keyman.PoolContainingCerts(certs...)
	if err != nil {
		log.Errorf("Could not create pool %v", err)
	}
	return
}

func displayVersion() {
	log.Debugf("---- flashlight version: %s, release: %s, build revision date: %s ----", common.Version, common.PackageVersion, common.RevisionDate)
}

func initContext(deviceID string, version string, revisionDate string, isPro func() bool) {
	// Using "application" allows us to distinguish between errors from the
	// lantern client vs other sources like the http-proxy, etop.
	ops.SetGlobal("app", "lantern-client")
	ops.SetGlobal("app_version", fmt.Sprintf("%v (%v)", version, revisionDate))
	ops.SetGlobal("go_version", runtime.Version())
	ops.SetGlobal("os_name", runtime.GOOS)
	ops.SetGlobal("os_arch", runtime.GOARCH)
	ops.SetGlobal("device_id", deviceID)
	ops.SetGlobalDynamic("geo_country", func() interface{} { return geolookup.GetCountry(0) })
	ops.SetGlobalDynamic("client_ip", func() interface{} { return geolookup.GetIP(0) })
	ops.SetGlobalDynamic("timezone", func() interface{} { return time.Now().Format("MST") })
	ops.SetGlobalDynamic("locale_language", func() interface{} {
		lang, _ := jibber_jabber.DetectLanguage()
		return lang
	})
	ops.SetGlobalDynamic("locale_country", func() interface{} {
		country, _ := jibber_jabber.DetectTerritory()
		return country
	})
	ops.SetGlobalDynamic("is_pro", func() interface{} {
		return isPro()
	})

	if osStr, err := osversion.GetHumanReadable(); err == nil {
		ops.SetGlobal("os_version", osStr)
	}
}
