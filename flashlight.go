package flashlight

import (
	"crypto/x509"
	"fmt"
	"math/rand"
	"path/filepath"
	"runtime"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight/geolookup"
	fops "github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/service"
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
	"github.com/getlantern/flashlight/proxied"

	// Make sure logging is initialized
	_ "github.com/getlantern/flashlight/logging"
)

var (
	log = golog.LoggerFor("flashlight")

	// 100% of the below ops are reported to borda, irrespective of the borda
	// sample percentage. This should all be low-volume operations, otherwise we
	// will utilize too much bandwidth on the client.
	FullyReportedOps = []string{"client_started", "client_stopped", "traffic", "catchall_fatal", "sysproxy_on", "sysproxy_off", "report_issue"}
)

func Register(httpProxyAddr string,
	socksProxyAddr string,
	useShortcut func() bool,
	useDetour func() bool,
	allowPrivateHosts func() bool,
	userConfig config.UserConfig,
	statsTracker common.StatsTracker,
) {
	service.GetRegistry().MustRegister(client.New,
		&client.ConfigOpts{
			UseShortcut:       useShortcut(),
			UseDetour:         useDetour(),
			AllowPrivateHosts: allowPrivateHosts(),
			ProToken:          userConfig.GetToken(),
			StatsTracker:      statsTracker,
			HTTPProxyAddr:     httpProxyAddr,
			Socks5ProxyAddr:   socksProxyAddr,
		},
		true,
		nil)
}

// Run runs a client proxy. It blocks as long as the proxy is running.
func Run(configDir string,
	autoReport func() bool,
	flagsAsMap map[string]interface{},
	beforeStart func() bool,
	afterStart func(),
	onConfigUpdate func(cfg *config.Global),
	userConfig config.UserConfig,
	onError func(err error),
	deviceID string) error {

	elapsed := mtime.Stopwatch()
	displayVersion()
	initContext(deviceID, common.Version, common.RevisionDate)
	op := fops.Begin("client_started")

	cl, _ := service.GetRegistry().MustLookup(client.ServiceType)
	ch := cl.Subscribe()
	beforeStart()
	go func() {
		for m := range ch {
			msg := m.(client.Message)
			if msg.ProxyType == client.HTTPProxy {
				proxied.SetProxyAddr(eventual.DefaultGetter(msg.Addr))
				log.Debug("Started client HTTP proxy")
				op.SetMetricSum("startup_time", float64(elapsed().Seconds()))
				geoService, geo := service.GetRegistry().MustLookup(geolookup.ServiceType)
				onGeo := geoService.Subscribe()
				geo.(*geolookup.GeoLookup).Refresh()
				ops.Go(func() {
					// wait for geo info before reporting so that we know the client ip and
					// country
					select {
					case <-onGeo:
						// okay, we've got geolocation info
					case <-time.After(5 * time.Minute):
						// failed to get geolocation info within 5 minutes, just record end of
						// startup anyway
					}
					op.End()
				})
				afterStart()
			}
		}
	}()

	proxiesDispatch := func(conf interface{}) {
		proxyMap := conf.(map[string]*chained.ChainedServerInfo)
		if len(proxyMap) > 0 {
			log.Debugf("Applying proxy config with proxies: %v", proxyMap)
			cl.MustReconfigure(service.ConfigUpdates{
				"Proxies":  proxyMap,
				"DeviceID": deviceID,
			})
		}
	}
	globalDispatch := func(conf interface{}) {
		// Don't love the straight cast here, but we're also the ones defining
		// the type in the factory method above.
		cfg := conf.(*config.Global)
		log.Debugf("Applying global config")
		applyClientConfig(cfg, deviceID, autoReport)
		onConfigUpdate(cfg)
	}
	config.Init(configDir, flagsAsMap, userConfig, proxiesDispatch, globalDispatch)

	return nil
}

func applyClientConfig(cfg *config.Global, deviceID string, autoReport func() bool) {
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

func initContext(deviceID string, version string, revisionDate string) {
	// Using "application" allows us to distinguish between errors from the
	// lantern client vs other sources like the http-proxy, etop.
	ops.SetGlobal("app", "lantern-client")
	ops.SetGlobal("app_version", fmt.Sprintf("%v (%v)", version, revisionDate))
	ops.SetGlobal("go_version", runtime.Version())
	ops.SetGlobal("os_name", runtime.GOOS)
	ops.SetGlobal("os_arch", runtime.GOARCH)
	ops.SetGlobal("device_id", deviceID)
	// TODO: add back setting geo info in flashlight
	ops.SetGlobalDynamic("timezone", func() interface{} { return time.Now().Format("MST") })
	ops.SetGlobalDynamic("locale_language", func() interface{} {
		lang, _ := jibber_jabber.DetectLanguage()
		return lang
	})
	ops.SetGlobalDynamic("locale_country", func() interface{} {
		country, _ := jibber_jabber.DetectTerritory()
		return country
	})

	if osStr, err := osversion.GetHumanReadable(); err == nil {
		ops.SetGlobal("os_version", osStr)
	}
}
