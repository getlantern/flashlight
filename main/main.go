// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	"context"
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/getlantern/golog"
	"github.com/getlantern/i18n"

	"contrib.go.opencensus.io/exporter/jaeger"
	"go.opencensus.io/trace"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/desktop"
)

var (
	log = golog.LoggerFor("flashlight")
)

func main() {
	// systray requires the goroutine locked with main thread, or the whole
	// application will crash.
	runtime.LockOSThread()
	// Since Go 1.6, panic prints only the stack trace of current goroutine by
	// default, which may not reveal the root cause. Switch to all goroutines.
	debug.SetTraceback("all")
	parseFlags()

	initTracing()
	ctx, span := trace.StartSpan(context.Background(), "lanternBackground")

	a := &desktop.App{
		ShowUI: !*headless,
		Flags:  flagsAsMap(),
		Ctx:    ctx,
	}
	a.Init()

	a.AddExitFunc("Ending background tracing span", func() {
		span.End()
	})

	if *help {
		flag.Usage()
		log.Fatal("Wrong arguments")
	}

	if *pprofAddr != "" {
		go func() {
			log.Debugf("Starting pprof page at http://%s/debug/pprof", *pprofAddr)
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
		log.Debug("Running headless")
		runApp(a)
		err := a.WaitForExit()
		if err != nil {
			log.Error(err)
		}
		log.Debug("Lantern stopped")
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
	log.Debugf("Using locale: %v", locale)
	if _, err := i18n.SetLocale(locale); err != nil {
		log.Debugf("i18n.SetLocale(%s) failed, fallback to OS default: %q", locale, err)

		// On startup GetLangauge will return '', as the browser has not set the language yet.
		// We use the OS locale instead and make sure the language is populated.
		if locale, err := i18n.UseOSLocale(); err != nil {
			log.Debugf("i18n.UseOSLocale: %q", err)
		} else {
			a.SetLanguage(locale)
		}
	}
}

type nullCloser struct{}

func (*nullCloser) Close() error { return nil }

func initTracing() {
	agentEndpointURI := "localhost:6831"
	collectorEndpointURI := "http://localhost:14268/api/traces"

	je, err := jaeger.NewExporter(jaeger.Options{
		AgentEndpoint:     agentEndpointURI,
		CollectorEndpoint: collectorEndpointURI,
		ServiceName:       "lantern-client",
	})
	if err != nil {
		log.Fatalf("Failed to create the Jaeger exporter: %v", err)
	}

	trace.RegisterExporter(je)

	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
}

func parseFlags() {
	args := os.Args[1:]
	// On OS X, the first time that the program is run after download it is
	// quarantined.  OS X will ask the user whether or not it's okay to run the
	// program.  If the user says that it's okay, OS X will run the program but
	// pass an extra flag like -psn_0_1122578.  flag.Parse() fails if it sees
	// any flags that haven't been declared, so we remove the extra flag.
	if len(os.Args) == 2 && strings.HasPrefix(os.Args[1], "-psn") {
		log.Debugf("Ignoring extra flag %v", os.Args[1])
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
		log.Debugf("Got signal \"%s\", exiting...", s)
		a.Exit(nil)
	}()
}
