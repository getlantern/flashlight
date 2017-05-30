package flashlight

import (
	"crypto/x509"
	"fmt"
	"math/rand"
	"path/filepath"
	"runtime"
	"time"

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
	FullyReportedOps = []string{"client_started", "client_stopped", "traffic", "catchall_fatal", "sysproxy_on", "sysproxy_off", "sysproxy_clear", "report_issue"}
)

// ComposeServices register and subscribe the common services used by both
// desktop and mobile. It panics when any error happens.
//
// Note: Lantern client has the freedom to listen on a HTTP/SOCKS5 proxy
// address different from what passed in. Caller should subscribe to client to
// receive the final addresses.
func ComposeServices(
	httpProxyAddr string,
	socks5ProxyAddr string,
	deviceID string,
	allowPrivateHosts bool,
	settings common.Settings,
	userConfig common.UserConfig,
	statsTracker common.StatsTracker,
	flagsAsMap map[string]interface{},
) {

	elapsed := mtime.Stopwatch()
	displayVersion()
	initContext(deviceID)
	op := fops.Begin("client_started")

	service.MustRegister(client.New(
		httpProxyAddr,
		socks5ProxyAddr,
		deviceID,
		allowPrivateHosts,
		settings,
		userConfig,
		statsTracker),
		&client.ConfigOpts{})

	registerConfigService(flagsAsMap, userConfig)
	service.MustRegister(borda.New(FullyReportedOps), &borda.ConfigOpts{})
	service.MustSub(config.ServiceID, func(msg interface{}) {
		switch c := msg.(type) {
		case config.Proxies:
			log.Debugf("Applying proxy config with proxies: %v", c)
			service.MustConfigure(client.ServiceID, func(opts service.ConfigOpts) {
				opts.(*client.ConfigOpts).Proxies = c
			})
		case *config.Global:
			log.Debugf("Applying global config")
			service.MustConfigure(borda.ServiceID, func(opts service.ConfigOpts) {
				o := opts.(*borda.ConfigOpts)
				o.ReportInterval = c.BordaReportInterval
				o.ReportAllOps = settings.IsAutoReport() && rand.Float64() <= c.BordaSamplePercentage/100
			})
			certs, err := getTrustedCACerts(c)
			if err != nil {
				log.Errorf("Unable to get trusted ca certs, not configuring fronted: %s", err)
			} else if c.Client != nil {
				cacheFile := filepath.Join(flagsAsMap["configdir"].(string), "masquerade_cache")
				fronted.Configure(certs, c.Client.MasqueradeSets, cacheFile)
			}
		}
	})

	service.MustRegister(geolookup.New(), nil)
	service.MustSub(geolookup.ServiceID, func(m interface{}) {
		info := m.(*geolookup.GeoInfo)
		ip, country := info.GetIP(), info.GetCountry()
		ops.SetGlobal("geo_country", country)
		ops.SetGlobal("client_ip", ip)
		service.Configure(client.ServiceID, func(opts service.ConfigOpts) {
			opts.(*client.ConfigOpts).GeoCountry = country
		})
	})

	service.MustSub(client.ServiceID, func(m interface{}) {
		msg := m.(client.Message)
		if msg.ProxyType == client.HTTPProxy {
			proxied.SetProxyAddr(eventual.DefaultGetter(msg.Addr))
			log.Debug("Started client HTTP proxy")
			op.SetMetricSum("startup_time", float64(elapsed().Seconds()))
			onGeo := service.MustSubCh(geolookup.ServiceID)
			geo := service.MustLookup(geolookup.ServiceID)
			geo.(*geolookup.GeoLookup).Refresh()
			ops.Go(func() {
				// wait for geo info before reporting so that we know the
				// client ip and country
				select {
				case <-onGeo:
					log.Debug("Got geolocation")
				case <-time.After(5 * time.Minute):
					// failed to get geolocation info within 5 minutes,
					// just record end of startup anyway
				}
				op.End()
				log.Debug("Lantern client started")
			})
		}
	})
}

func registerConfigService(flagsAsMap map[string]interface{}, userConfig common.UserConfig) {
	opts := config.DefaultConfigOpts(flagsAsMap["configdir"].(string))
	if v, _ := flagsAsMap["cloudconfig"].(string); v != "" {
		opts.Proxies.ChainedURL = v
	}
	if v, _ := flagsAsMap["frontedconfig"].(string); v != "" {
		opts.Proxies.FrontedURL = v
	}
	if v, _ := flagsAsMap["stickyconfig"].(bool); v {
		opts.Sticky = v
	}
	if v, _ := flagsAsMap["readableconfig"].(bool); v {
		opts.Obfuscate = !v
	}
	opts.OverrideGlobal = func(gl *config.Global) {
		if v, _ := flagsAsMap["borda-report-interval"].(time.Duration); v > 0 {
			gl.BordaReportInterval = v
		}
		if v, _ := flagsAsMap["borda-sample-percentage"].(float64); v > 0 {
			gl.BordaSamplePercentage = v
		}
	}
	opts.UserConfig = userConfig
	service.MustRegister(config.New(opts), nil)
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

func initContext(deviceID string) {
	// Using "app" allows us to distinguish between errors from the lantern
	// client vs other sources like the http-proxy, etc.
	ops.SetGlobal("app", "lantern-client")
	ops.SetGlobal("app_version", fmt.Sprintf("%v (%v)", common.Version, common.RevisionDate))
	ops.SetGlobal("go_version", runtime.Version())
	ops.SetGlobal("os_name", runtime.GOOS)
	ops.SetGlobal("os_arch", runtime.GOARCH)
	ops.SetGlobal("device_id", deviceID)
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
