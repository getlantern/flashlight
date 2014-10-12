// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/flashlight/nattraversal"
	"github.com/getlantern/flashlight/server"
	"github.com/getlantern/flashlight/statreporter"
	"github.com/getlantern/flashlight/statserver"
	"github.com/getlantern/flashlight/util"
)

const (
	ETAG          = "ETag"
	IF_NONE_MATCH = "If-None-Match"
)

var (
	CLOUD_CONFIG_POLL_INTERVAL = 1 * time.Minute

	// Command-line Flags
	help      = flag.Bool("help", false, "Get usage help")
	parentPID = flag.Int("parentpid", 0, "the parent process's PID, used on Windows for killing flashlight when the parent disappears")

	configUpdates = make(chan *config.Config)

	lastCloudConfigETag = ""
)

func main() {
	cfg := configure()

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

// configure parses the command-line flags and binds the configuration YAML.
// If there's a problem with the provided flags, it prints usage to stdout and
// exits with status 1.
func configure() *config.Config {
	flag.Parse()
	var err error
	cfg, err := config.LoadFromDisk()
	if err != nil {
		log.Debugf("Error loading config, using default: %s", err)
	}
	cfg = cfg.ApplyFlags()
	if *help || cfg.Addr == "" || (cfg.Role != "server" && cfg.Role != "client") {
		flag.Usage()
		os.Exit(1)
	}

	err = cfg.SaveToDisk()
	if err != nil {
		log.Fatalf("Unable to save config: %s", err)
	}

	go fetchConfigUpdates(cfg)

	return cfg
}

func fetchConfigUpdates(cfg *config.Config) {
	nextCloud := nextCloudPoll()
	for {
		cloudDelta := nextCloud.Sub(time.Now())
		var err error
		var updated *config.Config
		select {
		case <-time.After(1 * time.Second):
			if cfg.HasChangedOnDisk() {
				updated, err = config.LoadFromDisk()
			}
		case <-time.After(cloudDelta):
			if cfg.CloudConfig != "" {
				updated, err = fetchCloudConfig(cfg)
				if updated == nil && err == nil {
					log.Debugf("Configuration unchanged in cloud at: %s", cfg.CloudConfig)
				}
			}
			nextCloud = nextCloudPoll()
		}
		if err != nil {
			log.Errorf("Error fetching config updates: %s", err)
		} else if updated != nil {
			cfg = updated
			configUpdates <- updated
		}
	}
}

func nextCloudPoll() time.Time {
	sleepTime := (CLOUD_CONFIG_POLL_INTERVAL.Nanoseconds() / 2) + rand.Int63n(CLOUD_CONFIG_POLL_INTERVAL.Nanoseconds())
	return time.Now().Add(time.Duration(sleepTime))
}

func fetchCloudConfig(cfg *config.Config) (*config.Config, error) {
	log.Debugf("Fetching cloud config from: %s", cfg.CloudConfig)
	// Try it unproxied first
	bytes, err := doFetchCloudConfig(cfg, "")
	if err != nil && cfg.IsDownstream() {
		// If that failed, try it proxied
		bytes, err = doFetchCloudConfig(cfg, cfg.Addr)
	}
	if err != nil {
		return nil, fmt.Errorf("Unable to read yaml from %s: %s", cfg.CloudConfig, err)
	}
	if bytes == nil {
		return nil, nil
	}
	log.Debugf("Merging cloud configuration")
	return cfg.UpdatedFrom(bytes)
}

func doFetchCloudConfig(cfg *config.Config, proxyAddr string) ([]byte, error) {
	client, err := util.HTTPClient(cfg.CloudConfigCA, proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("Unable to initialize HTTP client: %s", err)
	}
	log.Debugf("Checking for cloud configuration at: %s", cfg.CloudConfig)
	req, err := http.NewRequest("GET", cfg.CloudConfig, nil)
	if err != nil {
		return nil, fmt.Errorf("Unable to construct request for cloud config at %s: %s", cfg.CloudConfig, err)
	}
	if lastCloudConfigETag != "" {
		// Don't bother fetching if unchanged
		req.Header.Set(IF_NONE_MATCH, lastCloudConfigETag)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Unable to fetch cloud config at %s: %s", cfg.CloudConfig, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 304 {
		return nil, nil
	} else if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Unexpected response status: %d", resp.StatusCode)
	}
	lastCloudConfigETag = resp.Header.Get(ETAG)
	return ioutil.ReadAll(resp.Body)
}

// Runs the client-side proxy
func runClientProxy(cfg *config.Config) {
	client := &client.Client{
		Addr:         cfg.Addr,
		ReadTimeout:  0, // don't timeout
		WriteTimeout: 0,
	}

	if cfg.WaddellAddr != "" {
		nattraversal.UpdateWaddellConn(cfg.WaddellAddr, &cfg.Client.Peers)
	}

	// Configure client initially
	client.Configure(cfg.Client, nil)
	// Continually poll for config updates and update client accordingly
	go func() {
		for {
			cfg := <-configUpdates
			nattraversal.UpdateWaddellConn(cfg.WaddellAddr,
				&cfg.Client.Peers)
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

	if cfg.WaddellAddr != "" {
		nattraversal.UpdateWaddellConn(cfg.WaddellAddr, nil)
	}

	srv.Configure(cfg.Server)
	// Continually poll for config updates and update server accordingly
	go func() {
		for {
			cfg := <-configUpdates
			nattraversal.UpdateWaddellConn(cfg.WaddellAddr, nil)
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
