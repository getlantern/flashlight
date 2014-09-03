// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/flashlight/server"
	"github.com/getlantern/flashlight/statreporter"
	"github.com/getlantern/flashlight/statserver"
	"github.com/getlantern/go-igdman/igdman"
)

const (
	PORTMAP_FAILURE = 50
)

var (
	CLOUD_CONFIG_POLL_INTERVAL = 1 * time.Minute

	// Command-line Flags
	help      = flag.Bool("help", false, "Get usage help")
	parentPID = flag.Int("parentpid", 0, "the parent process's PID, used on Windows for killing flashlight when the parent disappears")

	configUpdates = make(chan *Config)
	configErrors  = make(chan error)
)

// configure parses the command-line flags and binds the configuration YAML.
// If there's a problem with the provided flags, it prints usage to stdout and
// exits with status 1.
func configure() bool {
	cfg := DefaultConfig()
	cfg.InitFlags()

	err := cfg.Bind(configUpdates, configErrors)
	if err != nil {
		log.Fatalf("Unable to bind config: %s", err)
	}

	flag.Parse()
	if *help || cfg.Addr == "" || (cfg.Role != "server" && cfg.Role != "client") {
		flag.Usage()
		os.Exit(1)
	}

	err = cfg.Save()
	if err != nil {
		log.Fatalf("Unable to save config: %s", err)
	}

	return true
}

func main() {
	configure()

	// Read first configuration
	cfg := <-configUpdates

	if cfg.CpuProfile != "" {
		startCPUProfiling(cfg.CpuProfile)
		defer stopCPUProfiling(cfg.CpuProfile)
	}

	if cfg.MemProfile != "" {
		defer saveMemProfile(cfg.MemProfile)
	}

	saveProfilingOnSigINT(cfg)

	if cfg.CloudConfig != "" {
		go pollCloudConfig(cfg)
	}

	log.Debugf("Running proxy")
	if cfg.IsDownstream() {
		runClientProxy(cfg)
	} else {
		runServerProxy(cfg)
	}
}

func pollCloudConfig(cfg *Config) {
	log.Debugf("Polling for cloud configuration at: %s", cfg.CloudConfig)
	for {
		fetchCloudConfig(cfg)
		// Wait a random amount of time around CLOUD_CONFIG_POLL_INTERVAL_SECONDS +- 50%
		sleepTime := (CLOUD_CONFIG_POLL_INTERVAL.Nanoseconds() / 2) + rand.Int63n(CLOUD_CONFIG_POLL_INTERVAL.Nanoseconds())
		time.Sleep(time.Duration(sleepTime))
	}
}

func fetchCloudConfig(cfg *Config) {
	updated, err := doFetchCloudConfig(cfg, false)
	if err != nil && cfg.IsDownstream() {
		updated, err = doFetchCloudConfig(cfg, true)
	}
	if err != nil {
		log.Errorf("Unable to read yaml from %s: %s", cfg.CloudConfig, err)
		return
	}
	log.Debugf("Merging cloud configuration")
	cfg.Merge(updated)
}

func doFetchCloudConfig(cfg *Config, tunnelThroughLocalProxy bool) ([]byte, error) {
	client := &http.Client{}
	if tunnelThroughLocalProxy {
		// Use a custom transport that falls back to going through the proxy
		client.Transport = &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				rawUrl := cfg.Addr
				noHostSpecified := len(strings.Split(rawUrl, ":")[0]) == 0
				if noHostSpecified {
					rawUrl = "127.0.0.1" + rawUrl
				}
				return url.Parse(rawUrl)
			},
		}
	}
	resp, err := http.Get(cfg.CloudConfig)
	if err != nil {
		return nil, fmt.Errorf("Unable to fetch cloud config at %s: %s", cfg.CloudConfig, err)
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

// Runs the client-side proxy
func runClientProxy(cfg *Config) {
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
func runServerProxy(cfg *Config) {
	useAllCores()

	if cfg.Portmap > 0 {
		log.Debugf("Attempting to map external port %d", cfg.Portmap)
		err := mapPort(cfg)
		if err != nil {
			log.Errorf("Unable to map external port: %s", err)
			os.Exit(PORTMAP_FAILURE)
		}
		log.Debugf("Mapped external port %d", cfg.Portmap)
	}

	srv := &server.Server{
		Addr:         cfg.Addr,
		ReadTimeout:  0, // don't timeout
		WriteTimeout: 0,
		Host:         cfg.AdvertisedHost,
		CertContext: &server.CertContext{
			PKFile:         cfg.InConfigDir("proxypk.pem"),
			ServerCertFile: cfg.InConfigDir("servercert.pem"),
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
	err := srv.ListenAndServe()
	if err != nil {
		log.Fatalf("Unable to run server proxy: %s", err)
	}
}

func mapPort(cfg *Config) error {
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

func saveProfilingOnSigINT(cfg *Config) {
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
