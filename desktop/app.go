// Package app implements the desktop application functionality of flashlight
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
	"github.com/getlantern/flashlight/shortcut"
	"github.com/getlantern/flashlight/ui"
	"github.com/getlantern/flashlight/ws"

	"github.com/getlantern/flashlight/desktop/bandwidth"
	"github.com/getlantern/flashlight/desktop/emailproxy"
	"github.com/getlantern/flashlight/desktop/location"
	"github.com/getlantern/flashlight/desktop/loconfscanner"
	"github.com/getlantern/flashlight/desktop/notifier"
	"github.com/getlantern/flashlight/desktop/settings"
	"github.com/getlantern/flashlight/desktop/sysproxy"
)

var (
	log = golog.LoggerFor("flashlight.desktop")

	settingsName        = "settings.yaml"
	settingsNameStaging = "settings-staging.yaml"

	elapsed func() time.Duration
)

func init() {
	elapsed = mtime.Stopwatch()

	autoupdate.Version = common.PackageVersion
	autoupdate.PublicKey = []byte(packagePublicKey)

	rand.Seed(time.Now().UnixNano())
}

// App is the core of the Lantern desktop application, in the form of a library.
type App struct {
	ShowUI       bool
	Flags        map[string]interface{}
	exitCh       chan error
	statsTracker *statsTracker

	exitOnce    sync.Once
	chExitFuncs chan func()
	settings    *settings.Settings
}

// Init initializes the App's state
func (app *App) Init() {
	golog.OnFatal(app.exitOnFatal)
	app.Flags["staging"] = common.Staging
	_, ok := app.Flags["configdir"].(string)
	if !ok {
		app.Flags["configdir"] = appdir.General("Lantern")
	}
	if common.Staging {
		settingsName = settingsNameStaging
	}
	app.settings = settings.New()
	app.settings.Reconfigure(nil, &settings.ConfigOpts{
		common.Version,
		common.RevisionDate,
		common.BuildDate,
		app.settingsPath(),
	})
	app.exitCh = make(chan error, 1)
	// use buffered channel to avoid blocking the caller of 'AddExitFunc'
	// the number 10 is arbitrary
	app.chExitFuncs = make(chan func(), 10)
	app.statsTracker = &statsTracker{}
}

// LogPanicAndExit logs a panic and then exits the application. This function
// is only used in the panicwrap parent process.
func (app *App) LogPanicAndExit(msg interface{}) {
	log.Fatal(fmt.Errorf("Uncaught panic: %v", msg))
	// Turn off system proxy on panic
	// Reload settings to make sure we have an up-to-date addr
	app.settings.Reconfigure(nil, &settings.ConfigOpts{common.Version,
		common.RevisionDate,
		common.BuildDate,
		app.settingsPath(),
	})
	p := sysproxy.New()
	p.Reconfigure(nil, &sysproxy.ConfigOpts{app.settings.GetAddr()})
	p.Stop()
}

func (app *App) exitOnFatal(err error) {
	_ = logging.Close()
	app.Exit(nil)
}

// Run starts the app. It will block until the app exits.
func (app *App) Run() error {
	golog.OnFatal(app.exitOnFatal)
	app.AddExitFunc(recordStopped)

	app.startProfiling()

	listenAddr := app.Flags["addr"].(string)
	if listenAddr == "" {
		listenAddr = app.settings.GetString(settings.SNAddr)
	}
	if listenAddr == "" {
		listenAddr = defaultHTTPProxyAddress
	}

	socksAddr := app.Flags["socksaddr"].(string)
	if socksAddr == "" {
		socksAddr = app.settings.GetString(settings.SNSOCKSAddr)
	}
	if socksAddr == "" {
		socksAddr = defaultSOCKSProxyAddress
	}

	uiaddr := app.Flags["uiaddr"].(string)
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
			p := sysproxy.New()
			p.Reconfigure(nil, &sysproxy.ConfigOpts{listenAddr})
			p.Stop()
		} else {
			log.Debugf("Can't clear proxy settings for: %v", listenAddr)
		}
		app.Exit(nil)
	}

	if uiaddr != "" {
		// Is something listening on that port?
		if showErr := app.showExistingUI(uiaddr); showErr == nil {
			log.Debug("Lantern already running, showing existing UI")
			app.Exit(nil)
		}
	}

	service.MustRegister(client.New,
		&client.ConfigOpts{
			UseShortcut: !app.settings.GetProxyAll(),
			UseDetour:   !app.settings.GetProxyAll(),
			// on desktop, we do not allow private hosts
			AllowPrivateHosts: false,
			StatsTracker:      app.statsTracker,
			HTTPProxyAddr:     listenAddr,
			Socks5ProxyAddr:   socksAddr,
			ProToken:          app.settings.GetToken(),
			DeviceID:          app.settings.GetDeviceID(),
		},
		true,
		nil)

	var startupURL string
	bootstrap, err := config.ReadBootstrapSettings()
	if err != nil {
		log.Debugf("Could not read bootstrap settings: %v", err)
	} else {
		startupURL = bootstrap.StartupUrl
	}

	log.Debugf("Starting client UI at %v", uiaddr)
	// ui will handle empty uiaddr correctly
	err = ui.Start(uiaddr, !app.ShowUI, startupURL, localHTTPToken(app.settings))
	if err != nil {
		app.Exit(fmt.Errorf("Unable to start UI: %s", err))
	}
	ui.Handle("/data", ws.StartUIChannel())

	if e := app.settings.StartService(); e != nil {
		app.Exit(fmt.Errorf("Unable to register settings service: %q", e))
	}
	app.settings.SetUIAddr(ui.GetUIAddr())

	log.Debug(app.Flags)
	if app.Flags["proxyall"].(bool) {
		// If proxyall flag was supplied, force proxying of all
		app.settings.SetProxyAll(true)
	}

	go func() {
		err := flashlight.Run(
			app.settings.IsAutoReport,
			app.Flags,
			app.beforeStart(listenAddr),
			app.afterStart,
			app.onConfigUpdate,
			app.Exit,
			app.settings.GetDeviceID())
		if err != nil {
			app.Exit(err)
			return
		}
	}()

	return app.waitForExit()
}

func (app *App) settingsPath() string {
	name := settingsName
	if common.Staging {
		name = settingsNameStaging
	}
	configDir := app.Flags["configdir"].(string)
	return filepath.Join(configDir, name)
}

func (app *App) startProfiling() {
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

func (app *App) beforeStart(listenAddr string) func() bool {
	return func() bool {
		log.Debug("Before start")
		err := app.statsTracker.StartService()
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

		geoService, _ := service.MustRegister(
			geolookup.New,
			nil, // no ConfigOpts for geolookup
			true,
			nil)

		service.MustRegister(
			analytics.New,
			&analytics.ConfigOpts{DeviceID: app.settings.GetDeviceID(), Version: common.Version, Enabled: app.settings.IsAutoReport()},
			true, // either true or false should be ok as the ConfigOpts won't be valid until reconfigured with IP
			service.Deps{geolookup.ServiceType: func(m service.Message, self service.Service) {
				info := m.(*geolookup.GeoInfo)
				self.MustReconfigure(func(opts service.ConfigOpts) {
					opts.(*analytics.ConfigOpts).IP = info.GetIP()
				})
			}})
		service.MustRegister(
			location.New,
			&location.ConfigOpts{},
			true,
			service.Deps{geolookup.ServiceType: func(m service.Message, self service.Service) {
				info := m.(*geolookup.GeoInfo)
				self.MustReconfigure(func(opts service.ConfigOpts) {
					opts.(*location.ConfigOpts).Code = info.GetCountry()
				})
			}})
		service.MustRegister(
			sysproxy.New,
			&sysproxy.ConfigOpts{},
			false,
			nil, // TODO: depends on settings and client
		)

		chGeoService := geoService.Subscribe()
		go func() {
			for m := range chGeoService {
				info := m.(*geolookup.GeoInfo)
				ip, country := info.GetIP(), info.GetCountry()
				shortcut.Configure(country)
				ops.SetGlobal("geo_country", country)
				ops.SetGlobal("client_ip", ip)
			}
		}()

		ch := service.Subscribe(client.ServiceType)
		go func() {
			for m := range ch {
				log.Debugf("Got message %+v", m)
				msg := m.(client.Message)
				switch msg.ProxyType {
				case client.HTTPProxy:
					log.Debugf("Got HTTP proxy address: %v", msg.Addr)
					app.settings.SetString(settings.SNAddr, msg.Addr)
					s, _ := service.MustLookup(sysproxy.ServiceType)
					s.MustReconfigure(func(opts service.ConfigOpts) {
						opts.(*sysproxy.ConfigOpts).ProxyAddr = msg.Addr
					})
					if app.settings.GetSystemProxy() {
						s.Start()
					}
					app.OnSettingChange(settings.SNSystemProxy, func(val interface{}) {
						enable := val.(bool)
						if enable {
							s.Start()
						} else {
							s.Stop()
						}
					})

				case client.Socks5Proxy:
					log.Debugf("Got Socks5 proxy address: %v", msg.Addr)
					app.settings.SetString(settings.SNSOCKSAddr, msg.Addr)
				}
			}
		}()

		service.StartAll()

		setupUserSignal()

		app.AddExitFunc(notifier.Start())
		app.AddExitFunc(loconfscanner.Scanner(
			4*time.Hour,
			app.isProUser,
			app.settings.GetLanguage,
			&pastAnnouncements{app.settings}))

		return true
	}
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
	app.OnSettingChange(settings.SNAutoLaunch, func(val interface{}) {
		enable := val.(bool)
		go launcher.CreateLaunchFile(enable)
	})

	if app.ShowUI && !app.Flags["startup"].(bool) {
		// Launch a browser window with Lantern but only after the pac
		// URL and the proxy server are all up and running to avoid
		// race conditions where we change the proxy setup while the
		// UI server and proxy server are still coming up.
		ui.Show()
	} else {
		log.Debugf("Not opening browser. Startup is: %v", app.Flags["startup"])
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
