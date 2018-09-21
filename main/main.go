// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/getlantern/i18n"
	"github.com/getlantern/zaplog"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/desktop"
)

var log = logging.LoggerFor("flashlight")

func main() {
	// systray requires the goroutine locked with main thread, or the whole
	// application will crash.
	runtime.LockOSThread()
	// Since Go 1.6, panic prints only the stack trace of current goroutine by
	// default, which may not reveal the root cause. Switch to all goroutines.
	debug.SetTraceback("all")

	parseFlags()

	a := &desktop.App{
		ShowUI: !*headless,
		Flags:  flagsAsMap(),
	}
	a.Init()

	if *help {
		flag.Usage()
		log.Fatal("Wrong arguments")
	}

	if *pprofAddr != "" {
		go func() {
			log.Infof("Starting pprof page at http://%s/debug/pprof", *pprofAddr)
			srv := &http.Server{
				Addr: *pprofAddr,
			}
			if err := srv.ListenAndServe(); err != nil {
				log.Error(err)
			}
		}()
	}

	if *forceProxyAddr != "" {
		chained.ForceProxy(*forceProxyAddr, *forceAuthToken)
	}

	if a.ShowUI {
		runOnSystrayReady(a, func() {
			runApp(a)
		})
	} else {
		log.Info("Running headless")
		runApp(a)
		err := a.WaitForExit()
		if err != nil {
			log.Error(err)
		}
		log.Info("Lantern stopped")
		os.Exit(0)
	}
}

func runApp(a *desktop.App) {
	// Schedule cleanup actions
	handleSignals(a)
	if a.ShowUI {
		i18nInit(a)
		go func() {
			if err := configureSystemTray(a); err != nil {
				return
			}
			a.OnSettingChange(desktop.SNLanguage, func(lang interface{}) {
				refreshSystray(lang.(string))
			})
		}()
	}

	a.Run()
}

func i18nInit(a *desktop.App) {
	i18n.SetMessagesFunc(func(filename string) ([]byte, error) {
		return a.GetTranslations(filename)
	})
	locale := a.GetLanguage()
	if err := i18n.SetLocale(locale); err != nil {
		log.Infof("i18n.SetLocale(%s) failed, fallback to OS default: %q", locale, err)
		if err := i18n.UseOSLocale(); err != nil {
			log.Infof("i18n.UseOSLocale: %q", err)
		}
	}
}

func parseFlags() {
	args := os.Args[1:]
	// On OS X, the first time that the program is run after download it is
	// quarantined.  OS X will ask the user whether or not it's okay to run the
	// program.  If the user says that it's okay, OS X will run the program but
	// pass an extra flag like -psn_0_1122578.  flag.Parse() fails if it sees
	// any flags that haven't been declared, so we remove the extra flag.
	if len(os.Args) == 2 && strings.HasPrefix(os.Args[1], "-psn") {
		log.Infof("Ignoring extra flag %v", os.Args[1])
		args = []string{}
	}
	// Note - we can ignore the returned error because CommandLine.Parse() will
	// exit if it fails.
	_ = flag.CommandLine.Parse(args)
}

// Handle system signals for clean exit
func handleSignals(a *desktop.App) {
	c := make(chan os.Signal, 1)
	signal.Notify(c,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		s := <-c
		log.Infof("Got signal \"%s\", exiting...", s)
		a.Exit(nil)
	}()
}
