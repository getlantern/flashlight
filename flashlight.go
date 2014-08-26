// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"

	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/flashlight/proxy"
	"github.com/getlantern/flashlight/statreporter"
	"github.com/getlantern/flashlight/statserver"
	"github.com/oxtoacart/go-igdman/igdman"
)

const (
	PORTMAP_FAILURE = 50
)

var (
	// Command-line Flags
	help = flag.Bool("help", false, "Get usage help")
	cfg  = config.Default()

	// flagsParsed is unused, this is just a trick to allow us to parse
	// command-line flags before initializing the other variables
	flagsParsed = parseFlags()
)

// parseFlags parses the command-line flags.  If there's a problem with the
// provided flags, it prints usage to stdout and exits with status 1.
func parseFlags() bool {
	cfg.InitFlags()
	cfg.Bind()
	flag.Parse()
	if *help || cfg.Addr == "" || (cfg.Role != "server" && cfg.Role != "client") || cfg.UpstreamHost == "" {
		flag.Usage()
		os.Exit(1)
	}
	return true
}

func main() {
	if cfg.CpuProfile != "" {
		startCPUProfiling(cfg.CpuProfile)
		defer stopCPUProfiling(cfg.CpuProfile)
	}

	if cfg.MemProfile != "" {
		defer saveMemProfile(cfg.MemProfile)
	}

	saveProfilingOnSigINT()

	// Set up the common ProxyConfig for clients and servers
	proxyConfig := proxy.ProxyConfig{
		Addr:              cfg.Addr,
		ShouldDumpHeaders: cfg.DumpHeaders,
		ReadTimeout:       0, // don't timeout
		WriteTimeout:      0,
	}

	log.Debugf("Running proxy")
	if cfg.IsDownstream() {
		runClientProxy(proxyConfig)
	} else {
		runServerProxy(proxyConfig)
	}
}

// Runs the client-side proxy
func runClientProxy(proxyConfig proxy.ProxyConfig) {
	client := &proxy.Client{
		ProxyConfig:  proxyConfig,
		UpstreamHost: cfg.UpstreamHost,
		UpstreamPort: cfg.UpstreamPort,
		MasqueradeAs: cfg.MasqueradeAs,
		RootCA:       cfg.RootCA,
	}
	err := client.Run()
	if err != nil {
		log.Fatalf("Unable to run client proxy: %s", err)
	}
}

// Runs the server-side proxy
func runServerProxy(proxyConfig proxy.ProxyConfig) {
	useAllCores()

	if cfg.Portmap > 0 {
		log.Debugf("Attempting to map external port %d", cfg.Portmap)
		err := mapPort()
		if err != nil {
			log.Errorf("Unable to map external port: %s", err)
			os.Exit(PORTMAP_FAILURE)
		}
		log.Debugf("Mapped external port %d", cfg.Portmap)
	}

	server := &proxy.Server{
		ProxyConfig: proxyConfig,
		Host:        cfg.UpstreamHost,
		CertContext: &proxy.CertContext{
			PKFile:         inConfigDir("proxypk.pem"),
			ServerCertFile: inConfigDir("servercert.pem"),
		},
	}
	if cfg.InstanceId != "" {
		// Report stats
		server.StatReporter = &statreporter.Reporter{
			InstanceId: cfg.InstanceId,
			Country:    cfg.Country,
		}
	}
	if cfg.StatsAddr != "" {
		// Serve stats
		server.StatServer = &statserver.Server{
			Addr: cfg.StatsAddr,
		}
	}
	err := server.Run()
	if err != nil {
		log.Fatalf("Unable to run server proxy: %s", err)
	}
}

// inConfigDir returns the path to the given filename inside of the configDir
// specified at the command line.
func inConfigDir(filename string) string {
	if cfg.ConfigDir == "" {
		return filename
	} else {
		if _, err := os.Stat(cfg.ConfigDir); err != nil {
			if os.IsNotExist(err) {
				// Create config dir
				if err := os.MkdirAll(cfg.ConfigDir, 0755); err != nil {
					log.Fatalf("Unable to create configDir at %s: %s", cfg.ConfigDir, err)
				}
			}
		}
		return fmt.Sprintf("%s%c%s", cfg.ConfigDir, os.PathSeparator, filename)
	}
}

func mapPort() error {
	parts := strings.Split(cfg.Addr, ":")

	internalPort, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("Unable to parse local port: ")
	}

	internalIP := parts[0]
	if internalIP == "" {
		internalIP, err = determineInternalIP()
		if err != nil {
			return fmt.Errorf("Unable to determine internal IP: %s", err)
		}
	}

	igd, err := igdman.NewIGD()
	if err != nil {
		return fmt.Errorf("Unable to get IGD: %s", err)
	}

	igd.RemovePortMapping(igdman.TCP, cfg.Portmap)
	err = igd.AddPortMapping(igdman.TCP, internalIP, internalPort, cfg.Portmap, 0)
	if err != nil {
		return fmt.Errorf("Unable to map port with igdman %d: %s", cfg.Portmap, err)
	}

	return nil
}

func determineInternalIP() (string, error) {
	conn, err := net.Dial("tcp", "s3.amazonaws.com:443")
	if err != nil {
		return "", fmt.Errorf("Unable to determine local IP: %s", err)
	}
	defer conn.Close()
	return strings.Split(conn.LocalAddr().String(), ":")[0], nil
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

func saveProfilingOnSigINT() {
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
