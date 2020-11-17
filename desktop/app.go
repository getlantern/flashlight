// Package desktop implements the desktop application functionality of flashlight
package desktop

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight/browsers/simbrowser"
	"github.com/getlantern/flashlight/proxied"

	"github.com/getlantern/golog"
	"github.com/getlantern/i18n"
	"github.com/getlantern/launcher"
	"github.com/getlantern/memhelper"
	notify "github.com/getlantern/notifier"
	"github.com/getlantern/profiling"
	"github.com/getsentry/sentry-go"

	"github.com/getlantern/flashlight"
	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/flashlight/autoupdate"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/borda"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/datacap"
	"github.com/getlantern/flashlight/geolookup"

	//"github.com/getlantern/flashlight/diagnostics/trafficlog"

	"github.com/getlantern/flashlight/email"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/notifier"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/ui"
	"github.com/getlantern/flashlight/ws"

	desktopReplica "github.com/getlantern/flashlight/desktop/replica"
	"github.com/getlantern/replica"
)

var (
	log        = golog.LoggerFor("flashlight.app")
	_settings  *Settings
	settingsMx sync.RWMutex

	startTime = time.Now()
)

func init() {
	autoupdate.Version = common.PackageVersion
	autoupdate.PublicKey = []byte(packagePublicKey)

	rand.Seed(time.Now().UnixNano())
}

func getSettings() *Settings {
	settingsMx.RLock()
	defer settingsMx.RUnlock()
	return _settings
}

func setSettings(newSettings *Settings) {
	settingsMx.Lock()
	_settings = newSettings
	settingsMx.Unlock()
}

// App is the core of the Lantern desktop application, in the form of a library.
type App struct {
	hasExited            int64
	fetchedGlobalConfig  int32
	fetchedProxiesConfig int32

	Flags        map[string]interface{}
	ConfigDir    string
	exited       eventual.Value
	gaSession    analytics.Session
	statsTracker *statsTracker

	muExitFuncs sync.RWMutex
	exitFuncs   []func()

	uiServerCh            chan *ui.Server
	ws                    ws.UIChannel
	chrome                chromeExtension
	chGlobalConfigChanged chan bool
	flashlight            *flashlight.Flashlight
	startReplica          sync.Once

	// If both the trafficLogLock and proxiesLock are needed, the trafficLogLock should be obtained
	// first. Keeping the order consistent avoids deadlocking.

	// Log of network traffic to and from the proxies. Used to attach packet capture files to
	// reported issues. Nil if traffic logging is not enabled.
	//trafficLog     *trafficlog.TrafficLog
	//trafficLogLock sync.RWMutex

	// proxies are tracked by the application solely for data collection purposes. This value should
	// not be changed, except by App.onProxiesUpdate. State-changing methods on the dialers should
	// not be called. In short, this slice and its elements should be treated as read-only.
	//proxies     []balancer.Dialer
	//proxiesLock sync.RWMutex
}

// Init initializes the App's state
func (app *App) Init() {
	golog.OnFatal(app.exitOnFatal)

	log.Debugf("Using configdir: %v", app.ConfigDir)

	app.Flags["staging"] = common.Staging

	app.uiServerCh = make(chan *ui.Server, 1)
	app.chrome = newChromeExtension()
	//app.chrome.install()
	settings := app.loadSettings()
	setSettings(settings)
	app.exited = eventual.NewValue()
	// overridden later if auto report is allowed
	app.gaSession = analytics.NullSession{}
	app.statsTracker = NewStatsTracker()
	app.chGlobalConfigChanged = make(chan bool, 1)
	pro.OnProStatusChange(func(isPro bool, yinbiEnabled bool) {
		app.statsTracker.SetIsPro(isPro)
		app.statsTracker.SetYinbiEnabled(yinbiEnabled)
	})
	settings.OnChange(SNDisconnected, func(disconnected interface{}) {
		isDisconnected := disconnected.(bool)
		app.statsTracker.SetDisconnected(isDisconnected)
	})
	datacap.AddDataCapListener(func(hitDataCap bool) {
		app.statsTracker.SetHitDataCap(hitDataCap)
	})
	app.ws = ws.NewUIChannel()
}

// loadSettings loads the initial settings at startup, either from disk or using defaults.
func (app *App) loadSettings() *Settings {
	path := filepath.Join(app.ConfigDir, "settings.yaml")
	if common.Staging {
		path = filepath.Join(app.ConfigDir, "settings-staging.yaml")
	}
	return loadSettingsFrom(common.Version, common.RevisionDate, common.BuildDate, path, app.chrome)
}

// LogPanicAndExit logs a panic and then exits the application. This function
// is only used in the panicwrap parent process.
func (app *App) LogPanicAndExit(msg string) {
	if ShouldReportToSentry() {
		sentry.ConfigureScope(func(scope *sentry.Scope) {
			scope.SetLevel(sentry.LevelFatal)
		})

		sentry.CaptureMessage(msg)
		if result := sentry.Flush(common.SentryTimeout); result == false {
			log.Error("Flushing to Sentry timed out")
		}
	}
	// No need to print error as child process has already done so
}

func (app *App) exitOnFatal(err error) {
	_ = logging.Close()
	app.Exit(err)
}

func (app *App) uiServer() *ui.Server {
	server := <-app.uiServerCh
	app.uiServerCh <- server
	return server
}

// Run starts the app. It will block until the app exits.
func (app *App) Run() {
	golog.OnFatal(app.exitOnFatal)

	memhelper.Track(15*time.Second, 15*time.Second)

	settings := getSettings()

	// Run below in separate goroutine as config.Init() can potentially block when Lantern runs
	// for the first time. User can still quit Lantern through systray menu when it happens.
	go func() {
		log.Debug(app.Flags)
		if app.Flags["proxyall"].(bool) {
			// If proxyall flag was supplied, force proxying of all
			settings.SetProxyAll(true)
		}

		listenAddr := app.Flags["addr"].(string)
		if listenAddr == "" {
			listenAddr = settings.getString(SNAddr)
		}
		if listenAddr == "" {
			listenAddr = defaultHTTPProxyAddress
		}

		socksAddr := app.Flags["socksaddr"].(string)
		if socksAddr == "" {
			socksAddr = settings.getString(SNSOCKSAddr)
		}
		if socksAddr == "" {
			socksAddr = defaultSOCKSProxyAddress
		}

		if timeout := app.Flags["timeout"].(time.Duration); timeout > 0 {
			go func() {
				time.AfterFunc(timeout, func() {
					app.Exit(errors.New("No succeeding proxy got after running for %v, global config fetched: %v, proxies fetched: %v",
						timeout, atomic.LoadInt32(&app.fetchedGlobalConfig) == 1, atomic.LoadInt32(&app.fetchedProxiesConfig) == 1))
				})
			}()
		}

		if app.Flags["initialize"].(bool) {
			app.statsTracker.AddListener(func(newStats stats.Stats) {
				if newStats.HasSucceedingProxy {
					log.Debug("Finished initialization")
					app.Exit(nil)
				}
			})
		}

		var err error
		app.flashlight, err = flashlight.New(
			app.ConfigDir,
			app.Flags["vpn"].(bool),
			func() bool { return settings.getBool(SNDisconnected) }, // check whether we're disconnected
			settings.GetProxyAll,
			func() bool { return false }, // on desktop, we do not allow private hosts
			settings.IsAutoReport,
			app.Flags,
			app.onConfigUpdate,
			app.onProxiesUpdate,
			settings,
			app.statsTracker,
			app.IsPro,
			settings.GetLanguage,
			func() string {
				isPro, statusKnown := isProUserFast()
				if (isPro || !statusKnown) && !common.ForceAds() {
					// pro user (or status unknown), don't ad swap
					return ""
				}
				return app.PlansURL()
			},
			func(addr string) string { return addr }, // no dnsgrab reverse lookups on desktop
		)
		if err != nil {
			app.Exit(err)
			return
		}
		app.beforeStart(listenAddr)

		chProStatusChanged := make(chan bool, 1)
		pro.OnProStatusChange(func(isPro bool, yinbiEnabled bool) {
			chProStatusChanged <- isPro
		})
		chUserChanged := make(chan bool, 1)
		settings.OnChange(SNUserID, func(v interface{}) {
			chUserChanged <- true
		})
		// Just pass all of the channels that should trigger re-evaluating which features
		// are enabled for this user, country, etc.
		app.startFeaturesService(geolookup.OnRefresh(), chUserChanged, chProStatusChanged, app.chGlobalConfigChanged)

		app.flashlight.Run(
			listenAddr,
			socksAddr,
			app.afterStart,
			func(err error) { _ = app.Exit(err) },
		)
	}()
}

// startFeaturesService starts a new features service that dispatches features to any relevant
// listeners.
func (app *App) startFeaturesService(chans ...<-chan bool) {
	if service, err := app.ws.Register("features", func(write func(interface{})) {
		enabledFeatures := app.flashlight.EnabledFeatures()
		log.Debugf("Sending features enabled to new client: %v", enabledFeatures)
		write(enabledFeatures)
	}); err != nil {
		log.Errorf("Unable to serve enabled features to UI: %v", err)
	} else {
		for _, ch := range chans {
			go func(c <-chan bool) {
				for range c {
					features := app.flashlight.EnabledFeatures()
					log.Debugf("EnabledFeatures: %v", features)
					app.checkForReplica(features)
					select {
					case service.Out <- features:
						// ok
					default:
						// don't block if no-one is listening
					}
				}
			}(ch)
		}
	}

}

func (app *App) beforeStart(listenAddr string) {
	log.Debug("Got first config")
	var cpuProf, memProf string
	if cpu, cok := app.Flags["cpuprofile"]; cok {
		cpuProf = cpu.(string)
	}
	if mem, cok := app.Flags["memprofile"]; cok {
		memProf = mem.(string)
	}
	if cpuProf != "" || memProf != "" {
		log.Debugf("Start profiling with cpu file %s and mem file %s", cpuProf, memProf)
		finishProfiling := profiling.Start(cpuProf, memProf)
		app.AddExitFunc("finish profiling", finishProfiling)
	}

	if err := setUpSysproxyTool(); err != nil {
		app.Exit(err)
	}

	var startupURL string
	bootstrap, err := config.ReadBootstrapSettings(app.ConfigDir)
	if err != nil {
		log.Debugf("Could not read bootstrap settings: %v", err)
	} else {
		startupURL = bootstrap.StartupUrl
	}

	settings := getSettings()
	uiaddr := app.Flags["uiaddr"].(string)
	if uiaddr == "" {
		// stick with the last one if not specified from command line.
		if uiaddr = settings.GetUIAddr(); uiaddr != "" {
			host, port, splitErr := net.SplitHostPort(uiaddr)
			if splitErr != nil {
				log.Errorf("Invalid uiaddr in settings: %s", uiaddr)
				uiaddr = ""
			}
			// To allow Edge to open the UI, we force the UI address to be
			// localhost if it's 127.0.0.1 (the default for previous versions).
			// We do the same for all platforms for simplicity though it's only
			// useful on Windows 10 and above.
			if host == "127.0.0.1" {
				uiaddr = "localhost:" + port
			}
		}
	}

	if app.Flags["clear-proxy-settings"].(bool) {
		// This is a workaround that attempts to fix a Windows-only problem where
		// Lantern was unable to clean the system's proxy settings before logging
		// off.
		//
		// See: https://github.com/getlantern/lantern/issues/2776
		log.Debug("Requested clearing of proxy settings")
		_, port, splitErr := net.SplitHostPort(listenAddr)
		if splitErr == nil && port != "0" {
			log.Debugf("Clearing system proxy settings for: %v", listenAddr)
			clearSysproxyFor(listenAddr)
		} else {
			log.Debugf("Can't clear proxy settings for: %v", listenAddr)
		}
		app.Exit(nil)
		os.Exit(0)
	}

	if uiaddr != "" {
		// Is something listening on that port?
		if showErr := app.showExistingUI(uiaddr); showErr == nil {
			log.Debug("Lantern already running, showing existing UI")
			app.Exit(nil)
			os.Exit(0)
		}
	}

	log.Debugf("Starting client UI at %v", uiaddr)

	standalone := app.Flags["standalone"] != nil && app.Flags["standalone"].(bool)
	authaddr := app.GetStringFlag("authaddr", common.AuthServerAddr)
	log.Debugf("Using auth server at %v", authaddr)
	yinbiaddr := app.GetStringFlag("yinbiaddr", common.YinbiServerAddr)
	log.Debugf("Using Yinbi server %s", yinbiaddr)
	// ui will handle empty uiaddr correctly
	uiServer, err := ui.StartServer(ui.ServerParams{
		AuthServerAddr:  authaddr,
		YinbiServerAddr: yinbiaddr,
		ExtURL:          startupURL,
		AppName:         common.AppName,
		RequestedAddr:   uiaddr,
		LocalHTTPToken:  app.localHttpToken(),
		Standalone:      standalone,
		Handlers: []*ui.PathHandler{
			&ui.PathHandler{Pattern: "/pro/", Handler: pro.APIHandler(settings)},
			&ui.PathHandler{Pattern: "/data", Handler: app.ws.Handler()},
		},
	})
	if err != nil {
		app.Exit(fmt.Errorf("Unable to start UI: %s", err))
		return
	}
	if app.ShouldShowUI() {
		go func() {
			if err := configureSystemTray(app); err != nil {
				log.Errorf("Unable to configure system tray: %s", err)
				return
			}
			app.OnSettingChange(SNLanguage, func(lang interface{}) {
				refreshSystray(lang.(string))
			})
		}()
	}
	app.uiServerCh <- uiServer

	if e := settings.StartService(app.ws); e != nil {
		app.Exit(fmt.Errorf("Unable to register settings service: %q", e))
		return
	}
	settings.SetUIAddr(uiServer.GetUIAddr())

	if err = app.statsTracker.StartService(app.ws); err != nil {
		log.Errorf("Unable to serve stats to UI: %v", err)
	}

	setupUserSignal(app.ws, app.Connect, app.Disconnect)

	err = datacap.ServeDataCap(app.ws, func() string {
		return app.AddToken("/img/lantern_logo.png")
	}, app.PlansURL, isProUser)
	if err != nil {
		log.Errorf("Unable to serve bandwidth to UI: %v", err)
	}
	err = app.serveEmailProxy(app.ws)
	if err != nil {
		log.Errorf("Unable to serve mandrill to UI: %v", err)
	}

	// Don't block on fetching the location for the UI.
	go serveLocation(app.ws)

	if settings.IsAutoReport() {
		app.gaSession = analytics.Start(settings.GetDeviceID(), common.Version)
		app.AddExitFunc("stopping analytics", app.gaSession.End)
		go func() {
			app.gaSession.SetIP(geolookup.GetIP(eventual.Forever))
		}()
	}
	app.AddExitFunc("stopping loconf scanner", LoconfScanner(app.ConfigDir, 4*time.Hour, isProUser, func() string {
		return app.AddToken("/img/lantern_logo.png")
	}))
	app.AddExitFunc("stopping notifier", notifier.NotificationsLoop(app.gaSession))
	app.OnStatsChange(func(newStats stats.Stats) {
		for _, alert := range newStats.Alerts {
			note := &notify.Notification{
				Title:      i18n.T("BACKEND_ALERT_TITLE"),
				Message:    i18n.T("status." + alert.Alert()),
				ClickLabel: i18n.T("BACKEND_CLICK_LABEL_HELP"),
				ClickURL:   alert.HelpURL,
			}
			_ = notifier.ShowNotification(note, "alert-prompt")
		}
	})
}

func (app *App) checkForReplica(features map[string]bool) {
	if val, ok := features[config.FeatureReplica]; ok && val {
		app.startReplica.Do(func() {
			log.Debug("Starting replica from app")
			replicaHandler, err := desktopReplica.NewHTTPHandler(
				app.ConfigDir,
				getSettings(),
				&replica.Client{
					HttpClient: &http.Client{
						Transport: proxied.AsRoundTripper(
							func(req *http.Request) (*http.Response, error) {
								chained, err := proxied.ChainedNonPersistent("")
								if err != nil {
									return nil, fmt.Errorf("connecting to proxy: %w", err)
								}
								return chained.RoundTrip(req)
							},
						),
					},
					Endpoint: replica.DefaultEndpoint,
				},
				app.gaSession,
				desktopReplica.DefaultNewHttpHandlerOpts(),
			)
			if err != nil {
				log.Errorf("error creating replica http server: %v", err)
				app.Exit(err)
				return
			}
			app.AddExitFunc("cleanup replica http server", replicaHandler.Close)

			// Need a trailing '/' to capture all sub-paths :|, but we don't want to strip the leading '/'
			// in their handlers.
			app.uiServer().Handle(
				"/replica/",
				http.StripPrefix(
					"/replica",
					replicaHandler,
				),
				false,
			)
		})
	}
}

// Connect turns on proxying
func (app *App) Connect() {
	app.gaSession.Event("systray-menu", "connect")
	ops.Begin("connect").End()
	getSettings().setBool(SNDisconnected, false)
}

// Disconnect turns off proxying
func (app *App) Disconnect() {
	app.gaSession.Event("systray-menu", "disconnect")
	ops.Begin("disconnect").End()
	getSettings().setBool(SNDisconnected, true)
}

// GetLanguage returns the user language
func (app *App) GetLanguage() string {
	return getSettings().GetLanguage()
}

// SetLanguage sets the user language
func (app *App) SetLanguage(lang string) {
	getSettings().SetLanguage(lang)
}

// GetStringFlag gets the app flag with the given name. If the flag
// is missing, defaultValue is used
func (app *App) GetStringFlag(name, defaultValue string) string {
	var val string
	if app.Flags[name] != nil {
		val = app.Flags[name].(string)
	}
	if val == "" {
		val = defaultValue
	}
	return val
}

// OnSettingChange sets a callback cb to get called when attr is changed from UI.
// When calling multiple times for same attr, only the last one takes effect.
func (app *App) OnSettingChange(attr SettingName, cb func(interface{})) {
	getSettings().OnChange(attr, cb)
}

// OnStatsChange adds a listener for Stats changes.
func (app *App) OnStatsChange(fn func(stats.Stats)) {
	app.statsTracker.AddListener(fn)
}

func (app *App) sysproxyOn() {
	if err := sysproxyOn(); err != nil {
		app.statsTracker.SetAlert(
			stats.FAIL_TO_SET_SYSTEM_PROXY, err.Error(), false)
	}
}

func (app *App) afterStart(cl *client.Client) {
	if getSettings().GetSystemProxy() {
		app.sysproxyOn()
	}

	app.OnSettingChange(SNSystemProxy, func(val interface{}) {
		enable := val.(bool)
		if enable {
			app.sysproxyOn()
		} else {
			sysproxyOff()
		}
	})

	app.OnSettingChange(SNAutoLaunch, func(val interface{}) {
		enable := val.(bool)
		go launcher.CreateLaunchFile(enable)
	})

	app.AddExitFunc("turning off system proxy", sysproxyOff)
	app.AddExitFunc("flushing to borda", borda.Flush)
	if app.ShouldShowUI() && !app.Flags["startup"].(bool) {
		// Launch a browser window with Lantern but only after the pac
		// URL and the proxy server are all up and running to avoid
		// race conditions where we change the proxy setup while the
		// UI server and proxy server are still coming up.
		app.uiServer().ShowRoot("startup", common.AppName, app.statsTracker)
	} else {
		log.Debugf("Not opening browser. Startup is: %v", app.Flags["startup"])
	}

	settings := getSettings()
	if addr, ok := client.Addr(6 * time.Second); ok {
		settings.setString(SNAddr, addr)
	} else {
		log.Errorf("Couldn't retrieve HTTP proxy addr in time")
	}
	if socksAddr, ok := client.Socks5Addr(6 * time.Second); ok {
		settings.setString(SNSOCKSAddr, socksAddr)
	} else {
		log.Errorf("Couldn't retrieve SOCKS proxy addr in time")
	}
	err := servePro(app.ws)
	if err != nil {
		log.Errorf("Unable to serve pro data to UI: %v", err)
	}
}

func (app *App) onConfigUpdate(cfg *config.Global, src config.Source) {
	if src == config.Fetched {
		app.gaSession.Event("config", "global-fetched")
		atomic.StoreInt32(&app.fetchedGlobalConfig, 1)
	}
	autoupdate.Configure(cfg.UpdateServerURL, cfg.AutoUpdateCA, func() string {
		return app.AddToken("/img/lantern_logo.png")
	})
	email.SetDefaultRecipient(cfg.ReportIssueEmail)
	app.chGlobalConfigChanged <- true
	if len(cfg.GlobalBrowserMarketShareData) > 0 {
		err := simbrowser.SetMarketShareData(
			cfg.GlobalBrowserMarketShareData, cfg.RegionalBrowserMarketShareData)
		if err != nil {
			log.Errorf("failed to set browser market share data: %v", err)
		}
	}
	//app.configureTrafficLog(*cfg)
}

func (app *App) onProxiesUpdate(proxies []balancer.Dialer, src config.Source) {
	if src == config.Fetched {
		app.gaSession.Event("config", "proxies-fetched")
		atomic.StoreInt32(&app.fetchedProxiesConfig, 1)
	}
	/*
		app.trafficLogLock.Lock()
		app.proxiesLock.Lock()
		app.proxies = proxies
		if app.trafficLog != nil {
			proxyAddresses := []string{}
			for _, p := range proxies {
				proxyAddresses = append(proxyAddresses, p.Addr())
			}
			if err := app.trafficLog.UpdateAddresses(proxyAddresses); err != nil {
				log.Errorf("failed to update traffic log addresses: %v", err)
			}
		}
		app.proxiesLock.Unlock()
		app.trafficLogLock.Unlock()
	*/
}

/*
func (app *App) configureTrafficLog(cfg config.Global) {
	app.trafficLogLock.Lock()
	app.proxiesLock.RLock()
	defer app.trafficLogLock.Unlock()
	defer app.proxiesLock.RUnlock()

	enableTrafficLog := false
	if app.Flags["force-traffic-log"].(bool) {
		enableTrafficLog = true
		// This flag is used in development to run the traffic log. We probably want to actually
		// capture some packets if this flag is set.
		if cfg.TrafficLogCaptureBytes == 0 {
			cfg.TrafficLogCaptureBytes = 10 * 1024 * 1024
		}
		if cfg.TrafficLogSaveBytes == 0 {
			cfg.TrafficLogSaveBytes = 10 * 1024 * 1024
		}
	} else {
		for _, platform := range cfg.TrafficLogPlatforms {
			enableTrafficLog = platform == common.Platform && rand.Float64() < cfg.TrafficLogPercentage
		}
	}

	mtuLimit := app.Flags["tl-mtu-limit"].(int)
	if mtuLimit == 0 {
		mtuLimit = trafficlog.MTULimitNone
	}

	switch {
	case enableTrafficLog && app.trafficLog == nil:
		log.Debug("Turning traffic log on")
		app.trafficLog = trafficlog.New(
			cfg.TrafficLogCaptureBytes,
			cfg.TrafficLogSaveBytes,
			&trafficlog.Options{
				MTULimit:       mtuLimit,
				MutatorFactory: new(trafficlog.AppStripperFactory),
			})
		// These goroutines will close when the traffic log is closed.
		go func() {
			for err := range app.trafficLog.Errors() {
				log.Debugf("Traffic log error: %v", err)
			}
		}()
		go func() {
			for stats := range app.trafficLog.Stats() {
				log.Debugf("Traffic log stats: %v", stats)
			}
		}()
		proxyAddrs := []string{}
		for _, p := range app.proxies {
			proxyAddrs = append(proxyAddrs, p.Addr())
		}
		if err := app.trafficLog.UpdateAddresses(proxyAddrs); err != nil {
			log.Debugf("Failed to start traffic logging for proxies: %v", err)
			app.trafficLog.Close()
			app.trafficLog = nil
		}

	case enableTrafficLog && app.trafficLog != nil:
		app.trafficLog.UpdateBufferSizes(cfg.TrafficLogCaptureBytes, cfg.TrafficLogSaveBytes)

	case !enableTrafficLog && app.trafficLog != nil:
		log.Debug("Turning traffic log off")
		if err := app.trafficLog.Close(); err != nil {
			log.Debugf("Failed to close traffic log (this will create a memory leak): %v", err)
		}
		app.trafficLog = nil
	}
}
*/

// showExistingUi triggers an existing Lantern running on the same system to
// open a browser to the Lantern start page.
func (app *App) showExistingUI(addr string) error {
	url := "http://" + addr + "/" + localHTTPToken(getSettings()) + "/startup"
	log.Debugf("Hitting local URL: %v", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Debugf("Could not build request: %s", err)
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Debugf("Could not hit local lantern: %s", err)
		return err
	}
	if resp.Body != nil {
		if err = resp.Body.Close(); err != nil {
			log.Debugf("Error closing body! %s", err)
		}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected response from existing Lantern: %d", resp.StatusCode)
	}
	return nil
}

// AddExitFunc adds a function to be called before the application exits.
func (app *App) AddExitFunc(label string, exitFunc func()) {
	app.muExitFuncs.Lock()
	app.exitFuncs = append(app.exitFuncs, func() {
		log.Debugf("Processing exit function: %v", label)
		exitFunc()
		log.Debugf("Done processing exit function: %v", label)
	})
	app.muExitFuncs.Unlock()
}

// Exit tells the application to exit, optionally supplying an error that caused
// the exit. Returns true if the app is actually exiting, false if exit has
// already been requested.
func (app *App) Exit(err error) bool {
	if atomic.CompareAndSwapInt64(&app.hasExited, 0, 1) {
		app.doExit(err)
		return true
	}
	return false
}

func (app *App) doExit(err error) {
	if err != nil {
		log.Errorf("Exiting app %d(%d) because of %v", os.Getpid(), os.Getppid(), err)
		if ShouldReportToSentry() {
			sentry.ConfigureScope(func(scope *sentry.Scope) {
				scope.SetLevel(sentry.LevelFatal)
			})

			sentry.CaptureException(err)
			if result := sentry.Flush(common.SentryTimeout); result == false {
				log.Error("Flushing to Sentry timed out")
			}
		}
	} else {
		log.Debugf("Exiting app %d(%d)", os.Getpid(), os.Getppid())
	}
	// call it before flushing borda (one of the exit funcs)
	recordStopped()
	defer func() {
		app.exited.Set(err)
		log.Debugf("Finished exiting app %d(%d)", os.Getpid(), os.Getppid())
	}()

	ch := make(chan struct{})
	go func() {
		app.runExitFuncs()
		close(ch)
	}()
	t := time.NewTimer(10 * time.Second)
	select {
	case <-ch:
		log.Debug("Finished running exit functions")
	case <-t.C:
		log.Debug("Timeout running exit functions, quit anyway")
	}
	if err := logging.Close(); err != nil {
		log.Errorf("Error closing log: %v", err)
	}
}

func (app *App) runExitFuncs() {
	var wg sync.WaitGroup
	// call plain exit funcs in parallel
	app.muExitFuncs.RLock()
	log.Debugf("Running %d exit functions", len(app.exitFuncs))
	wg.Add(len(app.exitFuncs))
	for _, f := range app.exitFuncs {
		go func(f func()) {
			f()
			wg.Done()
		}(f)
	}
	app.muExitFuncs.RUnlock()
	wg.Wait()
}

// WaitForExit waits for a request to exit the application.
func (app *App) WaitForExit() error {
	err, _ := app.exited.Get(-1)
	if err == nil {
		return nil
	}
	return err.(error)
}

// IsPro indicates whether or not the app is pro
func (app *App) IsPro() bool {
	isPro, _ := isProUserFast()
	return isPro
}

// ProxyAddrReachable checks if Lantern's HTTP proxy responds correct status
// within the deadline.
func (app *App) ProxyAddrReachable(ctx context.Context) error {
	req, err := http.NewRequest("GET", "http://"+getSettings().GetAddr(), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("Unexpected HTTP status %v", resp.StatusCode)
	}
	return nil
}

func recordStopped() {
	ops.Begin("client_stopped").
		SetMetricSum("uptime", time.Now().Sub(startTime).Seconds()).
		End()
}

// ShouldShowUI determines if we should show the UI or not.
func (app *App) ShouldShowUI() bool {
	return !app.Flags["headless"].(bool) && !app.Flags["initialize"].(bool)
}

// ShouldReportToSentry determines if we should report errors/panics to Sentry
func ShouldReportToSentry() bool {
	return !common.InDevelopment()
}

// OnTrayShow indicates the user has selected to show lantern from the tray.
func (app *App) OnTrayShow() {
	app.gaSession.Event("systray-menu", "show")
	app.uiServer().ShowRoot("show-"+common.AppName, "tray", app.statsTracker)
}

// OnTrayUpgrade indicates the user has selected to upgrade lantern from the tray.
func (app *App) OnTrayUpgrade() {
	app.gaSession.Event("systray-menu", "upgrade")
	app.uiServer().Show(app.PlansURL(), "proupgrade", "tray", app.statsTracker)
}

// PlansURL returns the URL for accessing the checkout/plans page directly.
func (app *App) PlansURL() string {
	return app.uiServer().AddToken("/") + "#/plans"
}

// AddToken adds our secure token to a given request path.
func (app *App) AddToken(path string) string {
	return app.uiServer().AddToken(path)
}

// GetTranslations adds our secure token to a given request path.
func (app *App) GetTranslations(filename string) ([]byte, error) {
	return ui.Translations(filename)
}

func (app *App) localHttpToken() string {
	if v, ok := app.Flags["noUiHttpToken"]; ok && v.(bool) {
		return ""
	}
	return localHTTPToken(getSettings())
}
