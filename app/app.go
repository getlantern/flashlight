// Package desktop implements the desktop application functionality of flashlight
package desktop

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/flashlight"
	"github.com/getlantern/golog"
	"github.com/getlantern/launcher"
	"github.com/getlantern/mtime"
	"github.com/getlantern/profiling"

	"github.com/getlantern/flashlight/analytics"
	"github.com/getlantern/flashlight/autoupdate"
	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxiedsites"
	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/flashlight/ui"
	"github.com/getlantern/flashlight/ws"

	"github.com/getlantern/flashlight/app/bandwidth"
	"github.com/getlantern/flashlight/app/emailproxy"
	"github.com/getlantern/flashlight/app/location"
	"github.com/getlantern/flashlight/app/loconfscanner"
	"github.com/getlantern/flashlight/app/notifier"
	"github.com/getlantern/flashlight/app/settings"
	"github.com/getlantern/flashlight/app/sysproxy"
)

var (
	log = golog.LoggerFor("flashlight.desktop")

	elapsed = mtime.Stopwatch()
)

const (
	settingsName        = "settings.yaml"
	settingsNameStaging = "settings-staging.yaml"
)

func init() {
	autoupdate.Version = common.PackageVersion
	autoupdate.PublicKey = []byte(packagePublicKey)

	rand.Seed(time.Now().UnixNano())
}

// App is the core of the Lantern desktop application, in the form of a library.
type App struct {
	// keep it public as the caller need access to it.
	Headless     bool
	flags        map[string]interface{}
	exitCh       chan error
	statsTracker *statsTracker
	exitOnce     sync.Once
	chExitFuncs  chan func()
	settings     *settings.Settings
}

// NewApp creates a new App instance given a set of flags
func NewApp(flags map[string]interface{}) *App {
	headless, _ := flags["headless"].(bool)
	app := &App{
		Headless: headless,
		flags:    flags,
		exitCh:   make(chan error, 1),
		// use buffered channel to avoid blocking the caller of 'AddExitFunc'
		// the number 50 is arbitrary
		chExitFuncs:  make(chan func(), 50),
		statsTracker: &statsTracker{},
	}
	golog.OnFatal(app.exitOnFatal)
	app.flags["staging"] = common.Staging
	configdir := app.flags["configdir"].(string)
	if configdir == "" {
		app.flags["configdir"] = appdir.General("Lantern")
	}
	app.settings = settings.New(
		common.Version,
		common.BuildDate,
		common.RevisionDate,
		app.settingsPath(),
	)
	if app.flags["proxyall"].(bool) {
		// If proxyall flag was supplied, force proxying of all
		app.settings.SetProxyAll(true)
	}
	log.Debugf("Created desktop app with flags %v", app.flags)
	return app
}

// LogPanicAndExit logs a panic and then exits the application. This function
// is only used in the panicwrap parent process.
func (app *App) LogPanicAndExit(msg interface{}) {
	// Turn off system proxy on panic
	// Reload settings to make sure we have an up-to-date addr
	s := settings.New(common.Version,
		common.RevisionDate,
		common.BuildDate,
		app.settingsPath(),
	)
	sysproxy.New(s.GetAddr()).Stop()
	log.Fatal(fmt.Errorf("Uncaught panic: %v", msg))
}

func (app *App) exitOnFatal(err error) {
	_ = logging.Close()
	app.Exit(nil)
}

// Run starts the app. It will block until the app exits.
func (app *App) Run() error {
	golog.OnFatal(app.exitOnFatal)
	app.AddExitFunc(recordStopped)

	listenAddr := app.flags["addr"].(string)
	if listenAddr == "" {
		listenAddr = app.settings.GetString(settings.SNAddr)
	}
	if listenAddr == "" {
		listenAddr = defaultHTTPProxyAddress
	}

	socksAddr := app.flags["socksaddr"].(string)
	if socksAddr == "" {
		socksAddr = app.settings.GetString(settings.SNSOCKSAddr)
	}
	if socksAddr == "" {
		socksAddr = defaultSOCKSProxyAddress
	}

	if app.flags["clear-proxy-settings"].(bool) {
		clearProxySetting(listenAddr)
		app.Exit(nil)
	}

	app.startProfiling()
	app.startUIServices()

	err := flashlight.Run(
		listenAddr,
		socksAddr,
		common.WrapSettings(
			common.Not(app.settings.GetProxyAll), // UseShortcut
			common.Not(app.settings.GetProxyAll), // UseDetour
			app.settings.IsAutoReport,
		),
		app.settings, // common.UserConfig
		app.statsTracker,
		app.flags,
		app.beforeStart,
		app.afterStart,
		app.Exit,
		app.settings.GetDeviceID())
	if err != nil {
		app.Exit(err)
	}

	service.Sub(config.ServiceType, func(msg interface{}) {
		switch c := msg.(type) {
		case *config.Global:
			proxiedsites.Configure(c.ProxiedSites)
			autoupdate.Configure(c.UpdateServerURL, c.AutoUpdateCA)
		}
	})
	return app.waitForExit()
}

func (app *App) beforeStart() bool {
	log.Debug("Before start")
	service.MustRegister(location.New(), &location.ConfigOpts{})
	service.MustRegister(
		loconfscanner.New(4*time.Hour, app.isProUser, &pastAnnouncements{app.settings}),
		&loconfscanner.ConfigOpts{Lang: app.settings.GetLanguage()})
	service.Sub(geolookup.ServiceType, func(m interface{}) {
		info := m.(*geolookup.GeoInfo)
		ip, country := info.GetIP(), info.GetCountry()
		service.MustConfigure(location.ServiceType, func(opts service.ConfigOpts) {
			opts.(*location.ConfigOpts).Code = country
		})
		service.MustConfigure(loconfscanner.ServiceType, func(opts service.ConfigOpts) {
			opts.(*loconfscanner.ConfigOpts).Country = country
		})
		ops.SetGlobal("geo_country", country)
		ops.SetGlobal("client_ip", ip)
	})

	service.Sub(client.ServiceType, func(m interface{}) {
		msg := m.(client.Message)
		switch msg.ProxyType {
		case client.HTTPProxy:
			log.Debugf("Got HTTP proxy address: %v", msg.Addr)
			app.settings.SetString(settings.SNAddr, msg.Addr)
			setProxy := app.settings.GetSystemProxy()
			service.MustRegister(sysproxy.New(msg.Addr),
				&sysproxy.ConfigOpts{setProxy})
			service.Start(sysproxy.ServiceType)
			setupUserSignal() // it depends on sysproxy service being registered.
			app.OnSettingChange(settings.SNSystemProxy, func(val interface{}) {
				service.Configure(sysproxy.ServiceType, func(o service.ConfigOpts) {
					o.(*sysproxy.ConfigOpts).Enable = val.(bool)
				})
			})

		case client.Socks5Proxy:
			log.Debugf("Got Socks5 proxy address: %v", msg.Addr)
			app.settings.SetString(settings.SNSOCKSAddr, msg.Addr)
		}
	})

	service.StartAll()

	app.AddExitFunc(notifier.Start())
	return true
}

func (app *App) startUIServices() {
	uiaddr := app.flags["uiaddr"].(string)
	if uiaddr == "" {
		// stick with the last one if not specified from command line.
		if uiaddr = app.settings.GetUIAddr(); uiaddr != "" {
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

	if uiaddr != "" {
		// Is something listening on that port?
		if showErr := app.showExistingUI(uiaddr); showErr == nil {
			log.Debug("Lantern already running, showing existing UI")
			app.Exit(nil)
		}
	}

	var startupURL string
	bootstrap, err := config.ReadBootstrapSettings()
	if err != nil {
		log.Debugf("Could not read bootstrap settings: %v", err)
	} else {
		startupURL = bootstrap.StartupUrl
	}

	log.Debugf("Starting client UI at %v", uiaddr)
	// ui will handle empty uiaddr correctly
	err = ui.Start(uiaddr, app.Headless, startupURL, localHTTPToken(app.settings))
	if err != nil {
		app.Exit(fmt.Errorf("Unable to start UI: %s", err))
	}
	app.settings.SetUIAddr(ui.GetUIAddr())
	ui.Handle("/data", ws.StartUIChannel())

	err = app.settings.StartService()
	if err != nil {
		app.Exit(fmt.Errorf("Unable to register settings service: %q", err))
	}
	app.OnSettingChange(settings.SNAutoLaunch, func(val interface{}) {
		enable := val.(bool)
		go launcher.CreateLaunchFile(enable)
	})

	err = app.statsTracker.StartService()
	if err != nil {
		log.Errorf("Unable to serve stats to UI: %v", err)
	}

	err = bandwidth.Start(app.isProUser)
	if err != nil {
		log.Errorf("Unable to serve bandwidth to UI: %v", err)
	}

	err = emailproxy.Start(app.settings,
		app.settings.GetDeviceID(),
		app.settings.GetString(settings.SNVersion),
		app.settings.GetString(settings.SNRevisionDate),
	)
	if err != nil {
		log.Errorf("Unable to serve mandrill to UI: %v", err)
	}
}

func (app *App) startProfiling() {
	var cpuProf, memProf string
	if cpu, cok := app.flags["cpuprofile"]; cok {
		cpuProf = cpu.(string)
	}
	if mem, cok := app.flags["memprofile"]; cok {
		memProf = mem.(string)
	}
	if cpuProf != "" || memProf != "" {
		log.Debugf("Start profiling with cpu file %s and mem file %s", cpuProf, memProf)
		finishProfiling := profiling.Start(cpuProf, memProf)
		app.AddExitFunc(finishProfiling)
	}
}

func (app *App) settingsPath() string {
	name := settingsName
	if common.Staging {
		name = settingsNameStaging
	}
	configDir := app.flags["configdir"].(string)
	return filepath.Join(configDir, name)
}

func (app *App) isProUser() (isPro bool, ok bool) {
	var userID int
	for {
		userID = int(app.settings.GetUserID())
		if userID > 0 {
			break
		}
		log.Debugf("Waiting for user ID to become non-zero")
		time.Sleep(10 * time.Second)
	}
	status, err := userStatus(app.settings.GetDeviceID(), userID, app.settings.GetToken())
	if err != nil {
		log.Errorf("Error getting user status? %v", err)
		return false, false
	}
	log.Debugf("User %d is '%v'", userID, status)
	return status == "active", true
}

// GetSetting gets the in memory setting with the name specified by attr
func (app *App) GetSetting(name settings.SettingName) interface{} {
	return app.settings.GetSetting(name)
}

// OnSettingChange sets a callback cb to get called when attr is changed from UI.
// When calling multiple times for same attr, only the last one takes effect.
func (app *App) OnSettingChange(attr settings.SettingName, cb func(interface{})) {
	app.settings.OnChange(attr, cb)
}

func (app *App) afterStart() {
	if app.Headless || app.flags["startup"].(bool) {
		log.Debugf("Not opening browser. Startup is: %v", app.flags["startup"])
	} else {
		// Launch a browser window with Lantern but only after the pac
		// URL and the proxy server are all up and running to avoid
		// race conditions where we change the proxy setup while the
		// UI server and proxy server are still coming up.
		ui.Show()
	}
	// register it until client is started because it requires proxied package
	// TODO: add explicit dependency to proxied package
	service.MustRegister(
		analytics.New(app.settings.IsAutoReport(), app.settings.GetDeviceID(), common.Version),
		&analytics.ConfigOpts{})
	service.Start(analytics.ServiceType)
	service.Sub(geolookup.ServiceType, func(m interface{}) {
		ip := m.(*geolookup.GeoInfo).GetIP()
		service.MustConfigure(analytics.ServiceType,
			func(opts service.ConfigOpts) {
				opts.(*analytics.ConfigOpts).GeoIP = ip
			})
	})
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
	app.exitOnce.Do(func() {
		app.doExit(err)
	})
}

func (app *App) doExit(err error) {
	log.Errorf("Exiting app because of %v", err)
	defer func() {
		service.StopAll()
		app.exitCh <- err
		log.Debug("Finished exiting app")
	}()
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

func recordStopped() {
	ops.Begin("client_stopped").
		SetMetricSum("uptime", elapsed().Seconds()).
		End()
}

type pastAnnouncements struct {
	s *settings.Settings
}

func (p *pastAnnouncements) Get() []string {
	return p.s.GetStringArray(settings.SNPastAnnouncements)
}

func (p *pastAnnouncements) Add(s string) {
	past := p.s.GetStringArray(settings.SNPastAnnouncements)
	past = append(past, s)
	p.s.SetStringArray(settings.SNPastAnnouncements, past)
}

// localHTTPToken fetches the local HTTP token from disk if it's there, and
// otherwise creates a new one and stores it.
func localHTTPToken(set *settings.Settings) string {
	tok := set.GetLocalHTTPToken()
	if tok == "" {
		t := ui.LocalHTTPToken()
		set.SetLocalHTTPToken(t)
		return t
	}
	return tok
}

func clearProxySetting(listenAddr string) {
	// This is a workaround that attempts to fix a Windows-only problem where
	// Lantern was unable to clean the system's proxy settings before logging
	// off.
	//
	// See: https://github.com/getlantern/lantern/issues/2776
	log.Debug("Requested clearing of proxy settings")
	_, port, splitErr := net.SplitHostPort(listenAddr)
	if splitErr == nil && port != "0" {
		log.Debugf("Clearing system proxy settings for: %v", listenAddr)
		sysproxy.New(listenAddr).Clear()
	} else {
		log.Debugf("Can't clear proxy settings for: %v", listenAddr)
	}
}
