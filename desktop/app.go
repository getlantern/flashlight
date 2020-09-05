// Package desktop implements the desktop application functionality of flashlight
package desktop

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/errors"
	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight"
	"github.com/getlantern/golog"
	"github.com/getlantern/i18n"
	"github.com/getlantern/launcher"
	"github.com/getlantern/memhelper"
	notify "github.com/getlantern/notifier"
	"github.com/getlantern/profiling"
	"github.com/getlantern/trafficlog"
	"github.com/getlantern/trafficlog-flashlight/tlproc"
	"github.com/getsentry/sentry-go"

	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/flashlight/autoupdate"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/borda"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/datacap"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/icons"

	"github.com/getlantern/flashlight/email"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/notifier"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/flashlight/stats"
	"github.com/getlantern/flashlight/ui"
	"github.com/getlantern/flashlight/ws"
)

const (
	SENTRY_DSN               = "https://f65aa492b9524df79b05333a0b0924c5@sentry.io/2222244"
	SENTRY_TIMEOUT           = time.Second * 30
	SENTRY_MAX_MESSAGE_CHARS = 8000
)

const (
	trafficlogStartTimeout   = 5 * time.Second
	trafficlogRequestTimeout = time.Second

	// This message is only displayed when the traffic log needs to be installed.
	trafficlogInstallPrompt = "Lantern needs your permission to install diagnostic tools"

	// An asset in the icons package.
	trafficlogInstallIcon = "connected_32.ico"

	// This file, in the config directory, holds a timestamp from the last failed installation.
	trafficlogLastFailedInstallFile = "tl_last_failed"
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
	hasExited            int64
	fetchedGlobalConfig  int32
	fetchedProxiesConfig int32

	Flags        map[string]interface{}
	exited       eventual.Value
	gaSession    analytics.Session
	statsTracker *statsTracker

	muExitFuncs sync.RWMutex
	exitFuncs   []func()

	uiServerCh chan *ui.Server
	ws         ws.UIChannel
	chrome     chromeExtension

	// If both the trafficLogLock and proxiesLock are needed, the trafficLogLock should be obtained
	// first. Keeping the order consistent avoids deadlocking.

	// Log of network traffic to and from the proxies. Used to attach packet capture files to
	// reported issues. Nil if traffic logging is not enabled.
	trafficLog     *tlproc.TrafficLogProcess
	trafficLogLock sync.RWMutex

	// proxies are tracked by the application solely for data collection purposes. This value should
	// not be changed, except by App.onProxiesUpdate. State-changing methods on the dialers should
	// not be called. In short, this slice and its elements should be treated as read-only.
	proxies     []balancer.Dialer
	proxiesLock sync.RWMutex
}

// Init initializes the App's state
func (app *App) Init() {
	golog.OnFatal(app.exitOnFatal)
	app.Flags["staging"] = common.Staging
	app.uiServerCh = make(chan *ui.Server, 1)
	app.chrome = newChromeExtension()
	//app.chrome.install()
	settings = app.loadSettings()
	app.exited = eventual.NewValue()
	// overridden later if auto report is allowed
	app.gaSession = analytics.NullSession{}
	app.statsTracker = NewStatsTracker()
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
	dir := app.Flags["configdir"].(string)
	if dir == "" {
		dir = appdir.General("Lantern")
	}
	path := filepath.Join(dir, "settings.yaml")
	if common.Staging {
		path = filepath.Join(dir, "settings-staging.yaml")
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
		if result := sentry.Flush(SENTRY_TIMEOUT); result == false {
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

		err := flashlight.Run(
			listenAddr,
			socksAddr,
			app.Flags["configdir"].(string),
			app.Flags["vpn"].(bool),
			func() bool { return settings.getBool(SNDisconnected) }, // check whether we're disconnected
			settings.GetProxyAll,
			func() bool { return false }, // on desktop, we do not allow private hosts
			settings.IsAutoReport,
			app.Flags,
			app.beforeStart(listenAddr),
			app.afterStart,
			app.onConfigUpdate,
			app.onProxiesUpdate,
			settings,
			app.statsTracker,
			func(err error) {
				app.Exit(err)
			},
			app.IsPro,
			settings.GetUserID,
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
			app.AddExitFunc("finish profiling", finishProfiling)
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
				os.Exit(0)
			}
		}

		log.Debugf("Starting client UI at %v", uiaddr)

		standalone := app.Flags["standalone"] != nil && app.Flags["standalone"].(bool)
		// ui will handle empty uiaddr correctly
		uiServer, err := ui.StartServer(uiaddr,
			startupURL,
			localHTTPToken(settings),
			standalone,
			&ui.PathHandler{Pattern: "/pro/", Handler: pro.APIHandler(settings)},
			&ui.PathHandler{Pattern: "/data", Handler: app.ws.Handler()},
		)
		if err != nil {
			app.Exit(fmt.Errorf("Unable to start UI: %s", err))
			return false
		}
		app.uiServerCh <- uiServer

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

		if e := settings.StartService(app.ws); e != nil {
			app.Exit(fmt.Errorf("Unable to register settings service: %q", e))
			return false
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

		app.AddExitFunc("stopping loconf scanner", LoconfScanner(4*time.Hour, isProUser, func() string {
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
		return true
	}
}

// Connect turns on proxying
func (app *App) Connect() {
	app.gaSession.Event("systray-menu", "connect")
	ops.Begin("connect").End()
	settings.setBool(SNDisconnected, false)
}

// Disconnect turns off proxying
func (app *App) Disconnect() {
	app.gaSession.Event("systray-menu", "disconnect")
	ops.Begin("disconnect").End()
	settings.setBool(SNDisconnected, true)
}

// GetLanguage returns the user language
func (app *App) GetLanguage() string {
	return settings.GetLanguage()
}

// SetLanguage sets the user language
func (app *App) SetLanguage(lang string) {
	settings.SetLanguage(lang)
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

func (app *App) sysproxyOn() {
	if err := sysproxyOn(); err != nil {
		app.statsTracker.SetAlert(
			stats.FAIL_TO_SET_SYSTEM_PROXY, err.Error(), false)
	}
}

func (app *App) afterStart(cl *client.Client) {
	if settings.GetSystemProxy() {
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
		app.uiServer().ShowRoot("startup", "lantern", app.statsTracker)
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
	go app.configureTrafficLog(*cfg)
}

func (app *App) onProxiesUpdate(proxies []balancer.Dialer, src config.Source) {
	if src == config.Fetched {
		app.gaSession.Event("config", "proxies-fetched")
		atomic.StoreInt32(&app.fetchedProxiesConfig, 1)
	}
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
}

// This should be run in an independent routine as it may need to install and block for a
// user-action granting permissions.
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
		if cfg.TrafficLog.CaptureBytes == 0 {
			cfg.TrafficLog.CaptureBytes = 10 * 1024 * 1024
		}
		if cfg.TrafficLog.SaveBytes == 0 {
			cfg.TrafficLog.SaveBytes = 10 * 1024 * 1024
		}
		// Use the most up-to-date binary in development.
		cfg.TrafficLog.Reinstall = true
		// Always try to install the traffic log in development.
		lastFailedPath, err := common.InConfigDir("", trafficlogLastFailedInstallFile)
		if err != nil {
			log.Debugf("Failed to create path to traffic log install-last-failed file: %v", err)
		} else if err := os.Remove(lastFailedPath); err != nil {
			log.Debugf("Failed to remove traffic log install-last-failed file: %v", err)
		}
	} else {
		for _, platform := range cfg.TrafficLog.Platforms {
			enableTrafficLog = platform == common.Platform && rand.Float64() < cfg.TrafficLog.PercentClients
		}
	}

	switch {
	case enableTrafficLog && app.trafficLog == nil:
		installDir := appdir.General("Lantern")
		log.Debugf("Installing traffic log if necessary in %s", installDir)
		u, err := user.Current()
		if err != nil {
			log.Errorf("Failed to look up current user for traffic log install: %w", err)
			return
		}

		var iconFile string
		icon, err := icons.Asset(trafficlogInstallIcon)
		if err != nil {
			log.Debugf("Unable to load prompt icon during traffic log install: %v", err)
		} else {
			iconFile = filepath.Join(os.TempDir(), "lantern_tlinstall.ico")
			if err := ioutil.WriteFile(iconFile, icon, 0644); err != nil {
				// Failed to save the icon file, just use no icon.
				iconFile = ""
			}
		}

		lastFailedPath, err := common.InConfigDir("", trafficlogLastFailedInstallFile)
		if err != nil {
			log.Errorf("Failed to create path to traffic log install-last-failed file: %v", err)
			return
		}
		lastFailedRaw, err := ioutil.ReadFile(lastFailedPath)
		if err != nil && !os.IsNotExist(err) {
			log.Errorf("Unable to open traffic log install-last-failed file: %v", err)
			return
		}
		if lastFailedRaw != nil {
			if cfg.TrafficLog.WaitTimeSinceFailedInstall == 0 {
				log.Debug("Aborting traffic log install; install previously failed")
				return
			}
			lastFailed := new(time.Time)
			if err := lastFailed.UnmarshalText(lastFailedRaw); err != nil {
				log.Errorf("Failed to parse traffic log install-last-failed file: %v", err)
				return
			}
			if time.Since(*lastFailed) < cfg.TrafficLog.WaitTimeSinceFailedInstall {
				log.Debugf(
					"Aborting traffic log install; last failed %v ago, wait time is %v",
					time.Since(*lastFailed), cfg.TrafficLog.WaitTimeSinceFailedInstall,
				)
				return
			}
		}

		// Note that this is a no-op if the traffic log is already installed.
		installOpts := tlproc.InstallOptions{Overwrite: cfg.TrafficLog.Reinstall}
		installErr := tlproc.Install(
			installDir, u.Username, trafficlogInstallPrompt, iconFile, &installOpts)
		if installErr != nil {
			b, err := time.Now().MarshalText()
			if err != nil {
				log.Errorf("Failed to marshal time for traffic log install-last-failed file: %v", err)
				return
			}
			if err := ioutil.WriteFile(lastFailedPath, b, 0644); err != nil {
				log.Errorf("Failed to write traffic log install-last-failed file: %v", err)
				return
			}
			log.Errorf("Failed to install traffic log: %v", installErr)
			return
		}

		log.Debug("Turning traffic log on")
		app.trafficLog, err = tlproc.New(
			cfg.TrafficLog.CaptureBytes,
			cfg.TrafficLog.SaveBytes,
			installDir,
			&tlproc.Options{
				Options: trafficlog.Options{
					MutatorFactory: new(trafficlog.AppStripperFactory),
				},
				StartTimeout:   trafficlogStartTimeout,
				RequestTimeout: trafficlogRequestTimeout,
			})
		if err != nil {
			log.Errorf("Failed to start traffic log process: %v", err)
			return
		}
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
		err := app.trafficLog.UpdateBufferSizes(cfg.TrafficLog.CaptureBytes, cfg.TrafficLog.SaveBytes)
		if err != nil {
			log.Debugf("Failed to update traffic log buffer sizes: %v", err)
		}

	case !enableTrafficLog && app.trafficLog != nil:
		log.Debug("Turning traffic log off")
		if err := app.trafficLog.Close(); err != nil {
			log.Errorf("Failed to close traffic log (this will create a memory leak): %v", err)
		}
		app.trafficLog = nil
	}
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
			if result := sentry.Flush(SENTRY_TIMEOUT); result == false {
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
	req, err := http.NewRequest("GET", "http://"+settings.GetAddr(), nil)
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
	app.uiServer().ShowRoot("show-lantern", "tray", app.statsTracker)
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
