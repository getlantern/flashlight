// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	_ "github.com/anacrolix/envpprof"
	"github.com/getlantern/appdir"
	"github.com/getlantern/golog"
	"github.com/getlantern/i18n"
	"github.com/mitchellh/panicwrap"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/desktop"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/sentry"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	log = golog.LoggerFor("flashlight.main")
)

func main() {
	// systray requires the goroutine locked with main thread, or the whole
	// application will crash.
	runtime.LockOSThread()
	// Since Go 1.6, panic prints only the stack trace of current goroutine by
	// default, which may not reveal the root cause. Switch to all goroutines.
	debug.SetTraceback("all")
	flags := parseFlags()

	cdir := configDir()
	a := &desktop.App{
		ConfigDir: cdir,
		Flags:     flags,
	}
	a.Init()

	logFile, err := logging.RotatedLogsUnder(appdir.Logs(common.AppName))
	if err != nil {
		log.Error(err)
		// Nothing we can do if fails to create log files, leave logFile nil so
		// the child process writes to standard outputs as usual.
	}
	defer logFile.Close()

	if logFile != nil {
		go func() {
			tk := time.NewTicker(time.Minute)
			for {
				<-tk.C
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := a.ProxyAddrReachable(ctx); err != nil {
					// Can restart child process for better resiliency, but
					// just print an error message for now to be safe.
					fmt.Fprintf(logFile, "********* ERROR: Lantern HTTP proxy not working properly: %v\n", err)
				} else {
					fmt.Fprintln(logFile, "DEBUG: Lantern HTTP proxy is working fine")
				}
				cancel()
			}
		}()
	}

	// This init needs to be called before the panicwrapper fork so that it has been
	// defined in the parent process
	if desktop.ShouldReportToSentry() {
		sentry.InitSentry(sentry.Opts{
			DSN:             common.SentryDSN,
			MaxMessageChars: common.SentryMaxMessageChars,
		})
	}

	// Disable panicwrap for cases either unnecessary or when the exit status
	// is desirable.
	if disablePanicWrap() {
		log.Debug("Not spawning child process via panicwrap")
	} else {
		// panicwrap works by re-executing the running program (retaining arguments,
		// environmental variables, etc.) and monitoring the stderr of the program.
		exitStatus, err := panicwrap.Wrap(
			&panicwrap.WrapConfig{
				Handler: a.LogPanicAndExit,
				// Just forward signals to the child process so that it can cleanup appropriately
				ForwardSignals: []os.Signal{
					syscall.SIGHUP,
					syscall.SIGTERM,
					syscall.SIGQUIT,
					os.Interrupt,
				},
				// Pipe child process output to log files instead of letting the
				// child to write directly because we want to capture anything
				// printed by go runtime and other libraries as well.
				Stdout: logging.NonStopWriteCloser(logFile, os.Stdout),
				Writer: logging.NonStopWriteCloser(logFile, os.Stderr), // standard error
			},
		)
		if err != nil {
			// Something went wrong setting up the panic wrapper. This won't be
			// captured by panicwrap. At this point, continue execution without
			// panicwrap support. There are known cases where panicwrap will fail
			// to fork, such as Windows GUI app
			log.Errorf("Error setting up panic wrapper: %v", err)
		} else {
			// If exitStatus >= 0, then we're the parent process.
			if exitStatus >= 0 {
				os.Exit(exitStatus)
			}
		}
	}
	// We're in the child (wrapped) process now

	golog.SetPrepender(logging.Timestamped)

	if viper.GetString("pprofAddr") != "" {
		go func() {
			log.Debugf("Starting pprof page at http://%s/debug/pprof", viper.GetString("pprofAddr"))
			srv := &http.Server{
				Addr: viper.GetString("pprofAddr"),
			}
			if err := srv.ListenAndServe(); err != nil {
				log.Error(err)
			}
		}()
	}

	if viper.GetString("forceProxyAddr") != "" {
		chained.ForceProxy(viper.GetString("forceProxyAddr"), viper.GetString("forceAuthToken"))
	}

	if viper.GetString("forceConfigCountry") != "" {
		log.Debugf("Will force config fetches to pretend client country is: %v", viper.GetString("forceConfigCountry"))
		config.ForceCountry(viper.GetString("forceConfigCountry"))
	}

	if a.ShouldShowUI() {
		i18nInit(a)
		desktop.RunOnSystrayReady(viper.GetBool("standalone"), a, func() {
			runApp(a)
		})
	} else {
		log.Debug("Running headless")
		runApp(a)
		err := a.WaitForExit()
		if err != nil {
			log.Errorf("Lantern stopped with error %v", err)
			os.Exit(-1)
		}
		log.Debug("Lantern stopped")
		os.Exit(0)
	}
}

func configDir() string {
	cdir := viper.GetString("configdir")
	if cdir == "" {
		cdir = appdir.General(common.AppName)
	}
	log.Debugf("Using config dir %v", cdir)
	if _, err := os.Stat(cdir); err != nil {
		if os.IsNotExist(err) {
			// Create config dir
			if err := os.MkdirAll(cdir, 0750); err != nil {
				log.Errorf("Unable to create configdir at %s: %s", configDir, err)
			}
		}
	}
	return cdir
}

func runApp(a *desktop.App) {
	// Schedule cleanup actions
	handleSignals(a)
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

		// On startup GetLanguage will return '', as the browser has not set the language yet.
		// We use the OS locale instead and make sure the language is populated.
		if locale, err := i18n.UseOSLocale(); err != nil {
			log.Debugf("i18n.UseOSLocale: %q", err)
		} else {
			a.SetLanguage(locale)
		}
	}
}

func parseFlags() map[string]interface{} {
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

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	viper.AutomaticEnv()
	_ = viper.BindPFlags(pflag.CommandLine)

	if viper.GetBool("help") {
		flag.Usage()
		log.Fatal("Wrong arguments")
	}

	// viper's AllSettings returns time.Duration as a string for some reason. Convert it.
	flags := viper.AllSettings()
	parsedTimeout, _ := time.ParseDuration(flags["timeout"].(string)) // error is ignored since flag already checks validity
	flags["timeout"] = parsedTimeout
	return flags
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
		desktop.QuitSystray(a)
	}()
}

func disablePanicWrap() bool {
	return viper.GetBool("headless") || viper.GetBool("initialize") || viper.GetDuration("timeout") > 0
}
