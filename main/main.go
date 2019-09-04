// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	"flag"
	"fmt"
	"io"
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
	jaeger "github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/desktop"

	"github.com/mitchellh/panicwrap"
)

var log = golog.LoggerFor("flashlight")

func main() {
	// systray requires the goroutine locked with main thread, or the whole
	// application will crash.
	runtime.LockOSThread()
	// Since Go 1.6, panic prints only the stack trace of current goroutine by
	// default, which may not reveal the root cause. Switch to all goroutines.
	debug.SetTraceback("all")
	parseFlags()

	traceCloser := initTracing(*trace)
	a := &desktop.App{
		ShowUI: !*headless,
		Flags:  flagsAsMap(),
	}
	a.Init()
	a.AddExitFunc("Ending background tracing span", func() {
		traceCloser.Close()
	})

	if !*trace {
		wrapperC := handleWrapperSignals(a)

		// environmental variables, etc.) and monitoring the stderr of the program.
		exitStatus, err := panicwrap.BasicWrap(
			func(output string) {
				a.LogPanicAndExit(output)
			})
		if err != nil {
			// Something went wrong setting up the panic wrapper. This won't be
			// captured by panicwrap
			// At this point, continue execution without panicwrap support. There
			// are known cases where panicwrap will fail to fork, such as Windows
			// GUI app
			log.Errorf("Error setting up panic wrapper: %v", err)
		} else {
			// If exitStatus >= 0, then we're the parent process.
			if exitStatus >= 0 {
				os.Exit(exitStatus)
			}
		}

		// We're in the child (wrapped) process
		// Stop wrapper signal handling
		signal.Stop(wrapperC)
	}

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

	if *forceConfigCountry != "" {
		log.Debugf("Will force config fetches to pretend client country is: %v", *forceConfigCountry)
		config.ForceCountry(*forceConfigCountry)
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

func initTracing(trace bool) io.Closer {
	if !trace {
		log.Debug("Not initializing tracing")
		return &nullCloser{}
	}
	log.Debug("Initializing tracing")
	cfg := jaegercfg.Configuration{
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jaegercfg.ReporterConfig{
			//CollectorEndpoint: "https://trace.lantern.io:8443/api/traces",
			//CollectorEndpoint: "http://104.131.222.209:14268/api/traces",
			//CollectorEndpoint: "http://104.131.222.209:8443/api/traces",

			CollectorEndpoint: "http://104.131.222.209:8080/api/traces",
		},
	}

	closer, err := cfg.InitGlobalTracer(
		"lantern",
		jaegercfg.Logger(jaegerlog.StdLogger),
		jaegercfg.Metrics(metrics.NullFactory),
	)
	if err != nil {
		log.Errorf("Could not initialize jaeger tracer: %s", err.Error())
		return &nullCloser{}
	}
	return closer
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

// Handle system signals in panicwrap wrapper process for clean exit
func handleWrapperSignals(a *desktop.App) chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGPIPE) // it's okay to trap SIGPIPE in the wrapper but not in the main process because we can get it from failed network connections
	go func() {
		s := <-c
		a.LogPanicAndExit(fmt.Sprintf("Panicwrapper received signal %v", s))
	}()
	return c
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
