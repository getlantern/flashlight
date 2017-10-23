// Package app implements the desktop application functionality of flashlight
package app

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight"
	"github.com/getlantern/golog"
	"github.com/getlantern/launcher"
	"github.com/getlantern/profiling"

	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/flashlight/autoupdate"
	"github.com/getlantern/flashlight/borda"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/notifier"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/flashlight/proxiedsites"
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/ui"
	"github.com/getlantern/flashlight/ws"
)

var (
	log      = golog.LoggerFor("flashlight.app")
	settings *Settings

	startTime = time.Now()
)

func init() {
	autoupdate.Version = common.PackageVersion
	autoupdate.PublicKey = []byte(packagePublicKey)

	rand.Seed(time.Now().UnixNano())
}

// App is the core of the Lantern desktop application, in the form of a library.
type App struct {
	hasExited int64

	ShowUI       bool
	Flags        map[string]interface{}
	exited       eventual.Value
	statsTracker *statsTracker

	chExitFuncs     chan func()
	chLastExitFuncs chan func()
	uiServer        *ui.Server
}

// Init initializes the App's state
func (app *App) Init() {
	golog.OnFatal(app.exitOnFatal)
	app.Flags["staging"] = common.Staging
	settings = loadSettings(common.Version, common.RevisionDate, common.BuildDate)
	app.exited = eventual.NewValue()
	// use buffered channel to avoid blocking the caller of 'AddExitFunc'
	// the number 100 is arbitrary
	app.chExitFuncs = make(chan func(), 100)
	app.chLastExitFuncs = make(chan func(), 100)
	app.statsTracker = NewStatsTracker()
	pro.OnProStatusChange(func(isPro bool) {
		app.statsTracker.SetIsPro(isPro)
	})
	settings.OnChange(SNDisconnected, func(disconnected interface{}) {
		isDisconnected := disconnected.(bool)
		app.statsTracker.SetDisconnected(isDisconnected)
	})
	addDataCapListener(func(hitDataCap bool) {
		app.statsTracker.SetHitDataCap(hitDataCap)
	})
	app.uiDomain = app.Flags["ui-domain"].(string)
}

// LogPanicAndExit logs a panic and then exits the application. This function
// is only used in the panicwrap parent process.
func (app *App) LogPanicAndExit(msg interface{}) {
	// Reload settings to make sure we have an up-to-date addr
	settings = loadSettings(common.Version, common.RevisionDate, common.BuildDate)
	log.Fatal(fmt.Errorf("Uncaught panic: %v", msg))
}

func (app *App) exitOnFatal(err error) {
	_ = logging.Close()
	app.Exit(err)
}

// Run starts the app. It will block until the app exits.
func (app *App) Run() {
	golog.OnFatal(app.exitOnFatal)
	app.AddExitFunc(recordStopped)

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
		uiDomain := app.Flags["ui-domain"].(string)

		err := flashlight.Run(
			listenAddr,
			socksAddr,
			app.Flags["configdir"].(string),
			func() bool { return settings.getBool(SNDisconnected) }, // check whether we're disconnected
			func() bool { return !settings.GetProxyAll() },          // use shortcut
			func() bool { return !settings.GetProxyAll() },          // use detour
			func() bool { return false },                            // on desktop, we do not allow private hosts
			settings.IsAutoReport,
			app.Flags,
			app.beforeStart(listenAddr),
			app.afterStart,
			app.onConfigUpdate,
			settings,
			app.statsTracker,
			func(err error) {
				app.Exit(err)
			},
			settings.GetDeviceID(),
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
			func() bool { return true },              // always allow ad blocking on desktop
			func(addr string) string { return addr }, // no dnsgrab reverse lookups on desktop
			app.uiFilter(uiDomain),
		)
		if err != nil {
			app.Exit(err)
			return
		}
	}()
}

func (app *App) beforeStart(listenAddr string) func() bool {
	return func() bool {
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
			app.AddExitFunc(finishProfiling)
		}

		if err := setUpSysproxyTool(); err != nil {
			app.Exit(err)
		}

		var startupURL string
		bootstrap, err := config.ReadBootstrapSettings()
		if err != nil {
			log.Debugf("Could not read bootstrap settings: %v", err)
		} else {
			startupURL = bootstrap.StartupUrl
		}

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
			}
		}

		uiDomain := app.Flags["ui-domain"].(string)
		log.Debugf("Starting client UI at %v", uiaddr)

		// ui will handle empty uiaddr correctly
		if app.uiServer, err = ui.StartServer(uiaddr,
			startupURL,
			localHTTPToken(settings),
			uiDomain,
			settings.GetSystemProxy,
			&ui.PathHandler{Pattern: "/pro/", Handler: pro.APIHandler(settings)},
			&ui.PathHandler{Pattern: "/data", Handler: ws.StartUIChannel()},
		); err != nil {
			app.Exit(fmt.Errorf("Unable to start UI: %s", err))
		}

		if e := settings.StartService(); e != nil {
			app.Exit(fmt.Errorf("Unable to register settings service: %q", e))
		}
		settings.SetUIAddr(app.uiServer.GetUIAddr())

		if err = app.statsTracker.StartService(); err != nil {
			log.Errorf("Unable to serve stats to UI: %v", err)
		}

		setupUserSignal(app.Connect, app.Disconnect)

		err = serveBandwidth(func() string {
			return app.AddToken("/img/lantern_logo.png")
		}, app.PlansURL)
		if err != nil {
			log.Errorf("Unable to serve bandwidth to UI: %v", err)
		}

		err = serveEmailProxy()
		if err != nil {
			log.Errorf("Unable to serve mandrill to UI: %v", err)
		}

		// Don't block on fetching the location for the UI.
		go serveLocation()

		// Only run analytics once on startup.
		if settings.IsAutoReport() {
			stopAnalytics := analytics.Start(settings.GetDeviceID(), common.Version)
			app.AddExitFunc(stopAnalytics)
		}

		app.AddExitFunc(LoconfScanner(4*time.Hour, isProUser, func() string {
			return app.AddToken("/img/lantern_logo.png")
		}))
		app.AddExitFunc(notifier.NotificationsLoop())

		return true
	}
}

// Connect turns on proxying
func (app *App) Connect() {
	ops.Begin("connect").End()
	settings.setBool(SNDisconnected, false)
}

// Disconnect turns off proxying
func (app *App) Disconnect() {
	ops.Begin("disconnect").End()
	settings.setBool(SNDisconnected, true)
}

// GetLanguage returns the user language
func (app *App) GetLanguage() string {
	return settings.GetLanguage()
}

// OnSettingChange sets a callback cb to get called when attr is changed from UI.
// When calling multiple times for same attr, only the last one takes effect.
func (app *App) OnSettingChange(attr SettingName, cb func(interface{})) {
	settings.OnChange(attr, cb)
}

// OnStatsChange adds a listener for Stats changes.
func (app *App) OnStatsChange(fn func(stats.Stats)) {
	app.statsTracker.AddListener(fn)
}

func (app *App) afterStart() {
	if settings.GetSystemProxy() {
		sysproxyOn()
	}

	app.OnSettingChange(SNSystemProxy, func(val interface{}) {
		enable := val.(bool)
		if enable {
			sysproxyOn()
		} else {
			sysproxyOff()
		}
	})

	app.OnSettingChange(SNAutoLaunch, func(val interface{}) {
		enable := val.(bool)
		go launcher.CreateLaunchFile(enable)
	})

	app.AddExitFunc(sysproxyOff)
	app.AddExitFunc(borda.Flush)
	if app.ShowUI && !app.Flags["startup"].(bool) {
		// Launch a browser window with Lantern but only after the pac
		// URL and the proxy server are all up and running to avoid
		// race conditions where we change the proxy setup while the
		// UI server and proxy server are still coming up.
		app.uiServer.ShowRoot("startup", "lantern")
	} else {
		log.Debugf("Not opening browser. Startup is: %v", app.Flags["startup"])
	}
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
	err := servePro()
	if err != nil {
		log.Errorf("Unable to serve pro data to UI: %v", err)
	}
}

func (app *App) onConfigUpdate(cfg *config.Global) {
	proxiedsites.Configure(cfg.ProxiedSites)
	autoupdate.Configure(cfg.UpdateServerURL, cfg.AutoUpdateCA)
}

// showExistingUi triggers an existing Lantern running on the same system to
// open a browser to the Lantern start page.
func (app *App) showExistingUI(addr string) error {
	url := "http://" + addr + "/" + localHTTPToken(settings) + "/startup"
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
func (app *App) AddExitFunc(exitFunc func()) {
	app.chExitFuncs <- exitFunc
}

// AddExitFuncToEnd adds a function to be called before the application exits
// but after exit functions added with AddExitFunc.
func (app *App) AddExitFuncToEnd(exitFunc func()) {
	app.chLastExitFuncs <- exitFunc
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
		log.Errorf("Exiting app because of %v", err)
	} else {
		log.Error("Exiting app")
	}
	defer func() {
		app.exited.Set(err)
		log.Debug("Finished exiting app")
	}()

	app.runExitFuncs()
	app.runLastExitFuncs()
}

func (app *App) runExitFuncs() {
	// call plain exit funcs in order
	for {
		select {
		case f := <-app.chExitFuncs:
			f()
		default:
			return
		}
	}
}

func (app *App) runLastExitFuncs() {
	// call last exit funcs in reverse order
	lastExitFuncs := app.collectLastExitFuncs()
	for i := len(lastExitFuncs) - 1; i >= 0; i-- {
		lastExitFuncs[i]()
	}
}

func (app *App) collectLastExitFuncs() []func() {
	lastExitFuncs := make([]func(), 0)
	for {
		select {
		case f := <-app.chLastExitFuncs:
			lastExitFuncs = append(lastExitFuncs, f)
		default:
			return lastExitFuncs
		}
	}
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

func recordStopped() {
	ops.Begin("client_stopped").
		SetMetricSum("uptime", time.Now().Sub(startTime).Seconds()).
		End()
}

// ShouldShowUI determines if we should show the UI or not.
func (app *App) ShouldShowUI() bool {
	return app.ShowUI
}

// OnTrayShow indicates the user has selected to show lantern from the tray.
func (app *App) OnTrayShow() {
	app.uiServer.ShowRoot("show-lantern", "tray")
}

// OnTrayUpgrade indicates the user has selected to upgrade lantern from the tray.
func (app *App) OnTrayUpgrade() {
	app.uiServer.Show(app.PlansURL(), "proupgrade", "tray")
}

// PlansURL returns the URL for accessing the checkout/plans page directly.
func (app *App) PlansURL() string {
	return app.uiServer.AddToken("/") + "#/plans"
}

// AddToken adds our secure token to a given request path.
func (app *App) AddToken(path string) string {
	return app.uiServer.AddToken(path)
}

// GetTranslations adds our secure token to a given request path.
func (app *App) GetTranslations(filename string) ([]byte, error) {
	return ui.Translations(filename)
}

// uiFilter serves requests over a configured domain from the local UI server.
// This allows Lantern to run over a more standard domain, which makes it more
// standard and compatible with things like A/B testing software.
func (app *App) uiFilter(uiDomain string) func(req *http.Request) (*http.Request, error) {
	return uiFilterWithAddr(uiDomain, func() string {
		if app.uiServer == nil {
			return ""
		}
		return app.uiServer.GetListenAddr()
	})
}

// uiFilterWithAddr serves requests over a configured domain from the local UI server.
// This allows Lantern to run over a more standard domain, which makes it more
// standard and compatible with things like A/B testing software.
// This version makes testinga  bit easier.
func uiFilterWithAddr(uiDomain string, listenAddr func() string) func(req *http.Request) (*http.Request, error) {
	return func(req *http.Request) (*http.Request, error) {
		if req.URL != nil && strings.HasPrefix(req.URL.Host, app.uiDomain) ||
			strings.HasPrefix(req.Host, app.uiDomain) {
			if req.Method == http.MethodConnect && req.URL != nil {
				req.URL.Host = listenAddr()
			}
			// It's not clear why CONNECT requests also need the host header set here,
			// but it doesn't work without it.
			req.Host = listenAddr()
			return req, nil
		}
		return req, nil
	}
}
