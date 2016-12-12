// Package app implements the desktop application functionality of flashlight
package app

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/getlantern/flashlight"
	"github.com/getlantern/golog"
	"github.com/getlantern/launcher"
	"github.com/getlantern/profiling"

	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/flashlight/autoupdate"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/proxiedsites"
	"github.com/getlantern/flashlight/ui"
)

var (
	log      = golog.LoggerFor("flashlight.app")
	settings *Settings
	// if true, run Lantern against our staging infrastructure. This is set by
	// the linker using -ldflags
	stagingMode = "false"
)

func init() {
	log.Debugf("****************************** stagingMode: %v", stagingMode)

	autoupdate.Version = flashlight.PackageVersion
	autoupdate.PublicKey = []byte(packagePublicKey)

	rand.Seed(time.Now().UnixNano())
}

// App is the core of the Lantern desktop application, in the form of a library.
type App struct {
	ShowUI      bool
	Flags       map[string]interface{}
	exitCh      chan error
	chExitFuncs chan func()
}

// Init initializes the App's state
func (app *App) Init() {
	staging, err := strconv.ParseBool(stagingMode)
	if err == nil {
		app.Flags["staging"] = staging
	} else {
		app.Flags["staging"] = false
	}
	settings = loadSettings(flashlight.Version, flashlight.RevisionDate, flashlight.BuildDate)
	app.exitCh = make(chan error, 1)
	// use buffered channel to avoid blocking the caller of 'AddExitFunc'
	// the number 10 is arbitrary
	app.chExitFuncs = make(chan func(), 10)
	setupUserSignal()
}

// LogPanicAndExit logs a panic and then exits the application.
func (app *App) LogPanicAndExit(msg string) {
	if err := logging.EnableFileLogging(""); err != nil {
		panic("Error initializing logging")
	}

	<-logging.Configure("",
		// Reporting to Loggly is not possible at this point
		func() bool { return false },
	)

	log.Error(msg)

	logging.Flush()
	_ = logging.Close()

	app.Exit(nil)
}

// Run starts the app. It will block until the app exits.
func (app *App) Run() error {
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

		err := flashlight.Run(
			listenAddr,
			socksAddr,
			app.Flags["configdir"].(string),
			app.Flags["stickyconfig"].(bool),
			settings.GetProxyAll,
			settings.IsAutoReport,
			app.Flags,
			app.beforeStart,
			app.afterStart,
			app.onConfigUpdate,
			settings,
			app.Exit,
			settings.GetDeviceID())
		if err != nil {
			app.Exit(err)
			return
		}
	}()

	return app.waitForExit()
}

func (app *App) beforeStart() bool {
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

	if err := setUpPacTool(); err != nil {
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
			if _, _, err := net.SplitHostPort(uiaddr); err != nil {
				log.Errorf("Invalid uiaddr in settings: %s", uiaddr)
				uiaddr = ""
			}
		}
	}

	if uiaddr != "" && app.Flags["clear-proxy-settings"].(bool) {
		// This is a workaround that attempts to fix a Windows-only problem where
		// Lantern was unable to clean the system's proxy settings before logging
		// off.
		//
		// See: https://github.com/getlantern/lantern/issues/2776
		url := fmt.Sprintf("http://%s/proxy_on.pac", uiaddr)
		log.Debugf("Will clear proxy settings if it is prefixed with %s", url)
		doPACOff(url)
		app.Exit(nil)
	}

	if uiaddr != "" {
		// Is something listening on that port?
		if err := app.showExistingUI(uiaddr); err == nil {
			log.Debug("Lantern already running, showing existing UI")
			app.Exit(nil)
		}
	}

	log.Debugf("Starting client UI at %v", uiaddr)
	// ui will handle empty uiaddr correctly
	err = ui.Start(uiaddr, !app.ShowUI, startupURL, localHTTPToken(settings))
	if err != nil {
		app.Exit(fmt.Errorf("Unable to start UI: %s", err))
	}

	settings.SetUIAddr(ui.GetDirectUIAddr())

	err = serveBandwidth()
	if err != nil {
		log.Errorf("Unable to serve bandwidth to UI: %v", err)
	}

	err = serveEmailProxy()
	if err != nil {
		log.Errorf("Unable to serve mandrill to UI: %v", err)
	}

	err = serveLocation()
	if err != nil {
		log.Errorf("Unable to serve location to UI: %v", err)
	}

	// Only run analytics once on startup.
	if settings.IsAutoReport() {
		stopAnalytics := analytics.Start(settings.GetDeviceID(), flashlight.Version)
		app.AddExitFunc(stopAnalytics)
	}
	watchDirectAddrs()

	return true
}

// localHTTPToken fetches the local HTTP token from disk if it's there, and
// otherwise creates a new one and stores it.
func localHTTPToken(set *Settings) string {
	tok := set.GetLocalHTTPToken()
	if tok == "" {
		t := ui.LocalHTTPToken()
		set.SetLocalHTTPToken(t)
		return t
	}
	return tok
}

// GetSetting gets the in memory setting with the name specified by attr
func (app *App) GetSetting(name SettingName) interface{} {
	if val, ok := settingMeta[name]; ok {
		switch val.sType {
		case stBool:
			return settings.getBool(name)
		case stNumber:
			return settings.getInt64(name)
		case stString:
			return settings.getString(name)
		}
	} else {
		log.Errorf("Looking for non-existent setting? %v", name)
	}

	// should never reach here.
	return nil
}

// OnSettingChange sets a callback cb to get called when attr is changed from UI.
// When calling multiple times for same attr, only the last one takes effect.
func (app *App) OnSettingChange(attr SettingName, cb func(interface{})) {
	settings.OnChange(attr, cb)
}

func (app *App) afterStart() {
	servePACFile()
	if settings.GetSystemProxy() {
		pacOn()
	}
	app.OnSettingChange(SNSystemProxy, func(val interface{}) {
		enable := val.(bool)
		if enable {
			pacOn()
		} else {
			pacOff()
		}
	})

	app.OnSettingChange(SNAutoLaunch, func(val interface{}) {
		enable := val.(bool)
		go launcher.CreateLaunchFile(enable)
	})

	app.AddExitFunc(pacOff)
	if app.ShowUI && !app.Flags["startup"].(bool) {
		// Launch a browser window with Lantern but only after the pac
		// URL and the proxy server are all up and running to avoid
		// race conditions where we change the proxy setup while the
		// UI server and proxy server are still coming up.
		ui.Show()
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
}

func (app *App) onConfigUpdate(cfg *config.Global) {
	proxiedsites.Configure(cfg.ProxiedSites)
	autoupdate.Configure(cfg.UpdateServerURL, cfg.AutoUpdateCA)
}

// showExistingUi triggers an existing Lantern running on the same system to
// open a browser to the Lantern start page.
func (app *App) showExistingUI(addr string) error {
	url := "http://" + addr + "/startup"
	log.Debugf("Hitting local URL: %v", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Debugf("Could not build request: %s", err)
		return err
	}
	req.Header.Set("Origin", url)

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

// Exit tells the application to exit, optionally supplying an error that caused
// the exit.
func (app *App) Exit(err error) {
	log.Errorf("Exiting app because of %v", err)
	defer func() { app.exitCh <- err }()
	for {
		select {
		case f := <-app.chExitFuncs:
			log.Debugf("Calling exit func")
			f()
		default:
			log.Debugf("No exit func remaining, exit now")
			return
		}
	}
}

// WaitForExit waits for a request to exit the application.
func (app *App) waitForExit() error {
	return <-app.exitCh
}
