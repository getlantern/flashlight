package config

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/globals"
	"github.com/getlantern/flashlight/server"
	"github.com/getlantern/flashlight/statreporter"
	"github.com/getlantern/flashlight/util"
	"github.com/getlantern/golog"
	"github.com/getlantern/yamlconf"
	"gopkg.in/getlantern/yaml.v1"
)

const (
	CloudConfigPollInterval = 1 * time.Minute

	cloudflare  = "cloudflare"
	etag        = "ETag"
	ifNoneMatch = "If-None-Match"
)

var (
	log                 = golog.LoggerFor("flashlight.config")
	m                   *yamlconf.Manager
	lastCloudConfigETag = ""
)

type Config struct {
	Version       int
	CloudConfig   string
	CloudConfigCA string
	Addr          string
	Role          string
	Country       string
	StatsAddr     string
	CpuProfile    string
	MemProfile    string
	WaddellCert   string
	Stats         *statreporter.Config
	Server        *server.ServerConfig
	Client        *client.ClientConfig
}

// Start starts the configuration system.
func Start(updateHandler func(updated *Config)) (*Config, error) {
	m = &yamlconf.Manager{
		FilePath:         InConfigDir("flashlight.yaml"),
		FilePollInterval: 1 * time.Second,
		ConfigServerAddr: *configaddr,
		EmptyConfig: func() yamlconf.Config {
			return &Config{}
		},
		OneTimeSetup: func(ycfg yamlconf.Config) error {
			cfg := ycfg.(*Config)
			return cfg.applyFlags()
		},
		CustomPoll: func(currentCfg yamlconf.Config) (mutate func(yamlconf.Config) error, waitTime time.Duration, err error) {
			cfg := currentCfg.(*Config)
			waitTime = cfg.cloudPollSleepTime()
			if cfg.CloudConfig == "" {
				// Config doesn't have a CloudConfig, just ignore
				mutate = func(ycfg yamlconf.Config) error {
					// do nothing
					return nil
				}
				return
			}

			var bytes []byte
			bytes, err = cfg.fetchCloudConfig()
			if err == nil {
				mutate = func(ycfg yamlconf.Config) error {
					log.Debugf("Merging cloud configuration")
					cfg := ycfg.(*Config)
					return cfg.updateFrom(bytes)
				}
			}
			return
		},
	}
	initial, err := m.Start()
	var cfg *Config
	if err == nil {
		cfg = initial.(*Config)
		go func() {
			// Read updates
			for {
				next := m.Next()
				nextCfg := next.(*Config)
				updateGlobals(nextCfg)
				updateHandler(nextCfg)
			}
		}()
	}
	return cfg, err
}

func updateGlobals(cfg *Config) {
	globals.Country = cfg.Country
	if cfg.WaddellCert != "" {
		globals.WaddellCert = cfg.WaddellCert
	}
}

// Update updates the configuration using the given mutator function.
func Update(mutate func(cfg *Config) error) error {
	return m.Update(func(ycfg yamlconf.Config) error {
		return mutate(ycfg.(*Config))
	})
}

// InConfigDir returns the path to the given filename inside of the configdir.
func InConfigDir(filename string) string {
	if *configdir == "" {
		return filename
	} else {
		if _, err := os.Stat(*configdir); err != nil {
			if os.IsNotExist(err) {
				// Create config dir
				if err := os.MkdirAll(*configdir, 0755); err != nil {
					log.Fatalf("Unable to create configdir at %s: %s", *configdir, err)
				}
			}
		}
		return fmt.Sprintf("%s%c%s", *configdir, os.PathSeparator, filename)
	}
}

// GetVersion implements the method from interface yamlconf.Config
func (cfg *Config) GetVersion() int {
	return cfg.Version
}

// SetVersion implements the method from interface yamlconf.Config
func (cfg *Config) SetVersion(version int) {
	cfg.Version = version
}

// ApplyDefaults implements the method from interface yamlconf.Config
//
// ApplyDefaults populates default values on a Config to make sure that we have
// a minimum viable config for running.  As new settings are added to
// flashlight, this function should be updated to provide sensible defaults for
// those settings.
func (cfg *Config) ApplyDefaults() {
	// Default country
	if cfg.Country == "" {
		cfg.Country = *country
	}

	// Make sure we always have a stats config
	if cfg.Stats == nil {
		cfg.Stats = &statreporter.Config{}
	}

	if cfg.Stats.StatshubAddr == "" {
		cfg.Stats.StatshubAddr = *statshubAddr
	}

	if cfg.Client != nil && cfg.Role == "client" {
		cfg.applyClientDefaults()
	}
}

func (cfg *Config) applyClientDefaults() {
	// Make sure we always have at least one masquerade set
	if cfg.Client.MasqueradeSets == nil {
		cfg.Client.MasqueradeSets = make(map[string][]*client.Masquerade)
	}
	if len(cfg.Client.MasqueradeSets) == 0 {
		cfg.Client.MasqueradeSets[cloudflare] = cloudflareMasquerades
	}

	// Make sure we always have at least one server
	if cfg.Client.Servers == nil {
		cfg.Client.Servers = make([]*client.ServerInfo, 0)
	}
	if len(cfg.Client.Servers) == 0 {
		cfg.Client.Servers = append(cfg.Client.Servers, &client.ServerInfo{
			Host:          "roundrobin.getiantem.org",
			Port:          443,
			MasqueradeSet: cloudflare,
			QOS:           10,
			Weight:        1000000,
		})
	}

	// Make sure all servers have a QOS and Weight configured
	for _, server := range cfg.Client.Servers {
		if server.QOS == 0 {
			server.QOS = 5
		}
		if server.Weight == 0 {
			server.Weight = 100
		}
		if server.RedialAttempts == 0 {
			server.RedialAttempts = 2
		}
	}

	// Sort servers so that they're always in a predictable order
	cfg.Client.SortServers()
}

func (cfg *Config) IsDownstream() bool {
	return cfg.Role == "client"
}

func (cfg *Config) IsUpstream() bool {
	return !cfg.IsDownstream()
}

func (cfg Config) cloudPollSleepTime() time.Duration {
	return time.Duration((CloudConfigPollInterval.Nanoseconds() / 2) + rand.Int63n(CloudConfigPollInterval.Nanoseconds()))
}

func (cfg Config) fetchCloudConfig() ([]byte, error) {
	log.Debugf("Fetching cloud config from: %s", cfg.CloudConfig)
	// Try it unproxied first
	bytes, err := cfg.doFetchCloudConfig("")
	if err != nil && cfg.IsDownstream() {
		// If that failed, try it proxied
		bytes, err = cfg.doFetchCloudConfig(cfg.Addr)
	}
	if err != nil {
		return nil, fmt.Errorf("Unable to read yaml from %s: %s", cfg.CloudConfig, err)
	}
	return bytes, err
}

func (cfg Config) doFetchCloudConfig(proxyAddr string) ([]byte, error) {
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
		req.Header.Set(ifNoneMatch, lastCloudConfigETag)
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
	lastCloudConfigETag = resp.Header.Get(etag)
	return ioutil.ReadAll(resp.Body)
}

// updateFrom creates a new Config by merging the given yaml into this Config.
// Any servers in the updated yaml replace ones in the original Config and any
// masquerade sets in the updated yaml replace ones in the original Config.
func (updated *Config) updateFrom(updateBytes []byte) error {
	err := yaml.Unmarshal(updateBytes, updated)
	if err != nil {
		return fmt.Errorf("Unable to unmarshal YAML for update: %s", err)
	}
	// Need to de-duplicate servers, since yaml appends them
	servers := make(map[string]*client.ServerInfo)
	for _, server := range updated.Client.Servers {
		servers[server.Host] = server
	}
	updated.Client.Servers = make([]*client.ServerInfo, len(servers))
	i := 0
	for _, server := range servers {
		updated.Client.Servers[i] = server
		i = i + 1
	}
	return nil
}
