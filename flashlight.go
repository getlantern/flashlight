// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	"flag"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/server"
	"github.com/getlantern/flashlight/statreporter"
	"github.com/getlantern/flashlight/statserver"
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("flashlight")

	// Command-line Flags
	help      = flag.Bool("help", false, "Get usage help")
	parentPID = flag.Int("parentpid", 0, "the parent process's PID, used on Windows for killing flashlight when the parent disappears")

	configUpdates = make(chan *config.Config)
)

func main() {
	flag.Parse()
	configUpdates = make(chan *config.Config)
	cfg, err := config.Start(func(updated *config.Config) {
		configUpdates <- updated
	})
	if err != nil {
		log.Fatalf("Unable to start configuration: %s", err)
	}

	if cfg.CpuProfile != "" {
		startCPUProfiling(cfg.CpuProfile)
		defer stopCPUProfiling(cfg.CpuProfile)
	}

	if cfg.MemProfile != "" {
		defer saveMemProfile(cfg.MemProfile)
	}

	saveProfilingOnSigINT(cfg)

	log.Debugf("Running proxy")
	if cfg.IsDownstream() {
		runClientProxy(cfg)
	} else {
		runServerProxy(cfg)
	}
}

// Runs the client-side proxy
func runClientProxy(cfg *config.Config) {
	client := &client.Client{
		Addr:         cfg.Addr,
		ReadTimeout:  0, // don't timeout
		WriteTimeout: 0,
	}

	// Configure client initially
	client.Configure(cfg.Client, nil)
	// Continually poll for config updates and update client accordingly
	go func() {
		for {
			cfg := <-configUpdates
			client.Configure(cfg.Client, nil)
		}
	}()

	err := client.ListenAndServe()
	if err != nil {
		log.Fatalf("Unable to run client proxy: %s", err)
	}
}

// Runs the server-side proxy
func runServerProxy(cfg *config.Config) {
	useAllCores()

	srv := &server.Server{
		Addr:         cfg.Addr,
		ReadTimeout:  0, // don't timeout
		WriteTimeout: 0,
		CertContext: &server.CertContext{
			PKFile:         config.InConfigDir("proxypk.pem"),
			ServerCertFile: config.InConfigDir("servercert.pem"),
		},
	}
	if cfg.InstanceId != "" {
		// Report stats
		srv.StatReporter = &statreporter.Reporter{
			InstanceId: cfg.InstanceId,
			Country:    cfg.Country,
		}
	}
	if cfg.StatsAddr != "" {
		// Serve stats
		srv.StatServer = &statserver.Server{
			Addr: cfg.StatsAddr,
		}
	}

	srv.Configure(cfg.Server)
	// Continually poll for config updates and update server accordingly
	go func() {
		for {
			cfg := <-configUpdates
			srv.Configure(cfg.Server)
		}
	}()

	err := srv.ListenAndServe()
	if err != nil {
		log.Fatalf("Unable to run server proxy: %s", err)
	}
}

func useAllCores() {
	numcores := runtime.NumCPU()
	log.Debugf("Using all %d cores on machine", numcores)
	runtime.GOMAXPROCS(numcores)
}

func startCPUProfiling(filename string) {
	f, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	pprof.StartCPUProfile(f)
	log.Debugf("Process will save cpu profile to %s after terminating", filename)
}

func stopCPUProfiling(filename string) {
	log.Debugf("Saving CPU profile to: %s", filename)
	pprof.StopCPUProfile()
}

func saveMemProfile(filename string) {
	f, err := os.Create(filename)
	if err != nil {
		log.Errorf("Unable to create file to save memprofile: %s", err)
		return
	}
	log.Debugf("Saving heap profile to: %s", filename)
	pprof.WriteHeapProfile(f)
	f.Close()
}

func saveProfilingOnSigINT(cfg *config.Config) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		if cfg.CpuProfile != "" {
			stopCPUProfiling(cfg.CpuProfile)
		}
		if cfg.MemProfile != "" {
			saveMemProfile(cfg.MemProfile)
		}
		os.Exit(2)
	}()
}
