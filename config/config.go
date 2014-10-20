package config

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/server"
	"github.com/getlantern/flashlight/util"
	"github.com/getlantern/golog"
	"github.com/getlantern/yamlconf"
	"gopkg.in/getlantern/yaml.v1"
)

const (
	CloudConfigPollInterval = 5 * time.Second

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
	InstanceId    string
	StatsAddr     string
	Country       string
	CpuProfile    string
	MemProfile    string
	Server        *server.ServerConfig
	Client        *client.ClientConfig
}

var (
	// Flags
	configdir      = flag.String("configdir", "", "directory in which to store configuration, including flashlight.yaml (defaults to current directory)")
	configaddr     = flag.String("configaddr", "", "if specified, run an http-based configuration server at this address")
	cloudconfig    = flag.String("cloudconfig", "", "optional http(s) URL to a cloud-based source for configuration updates")
	cloudconfigca  = flag.String("cloudconfigca", "", "optional PEM encoded certificate used to verify TLS connections to fetch cloudconfig")
	addr           = flag.String("addr", "", "ip:port on which to listen for requests. When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https (required)")
	role           = flag.String("role", "", "either 'client' or 'server' (required)")
	instanceid     = flag.String("instanceid", "", "instanceId under which to report stats to statshub. If not specified, no stats are reported.")
	statsaddr      = flag.String("statsaddr", "", "host:port at which to make detailed stats available using server-sent events (optional)")
	country        = flag.String("country", "xx", "2 digit country code under which to report stats. Defaults to xx.")
	cpuprofile     = flag.String("cpuprofile", "", "write cpu profile to given file")
	memprofile     = flag.String("memprofile", "", "write heap profile to given file")
	portmap        = flag.Int("portmap", 0, "try to map this port on the firewall to the port on which flashlight is listening, using UPnP or NAT-PMP. If mapping this port fails, flashlight will exit with status code 50")
	advertisedHost = flag.String("server", "", "FQDN of flashlight server when running in server mode (required)")
	waddelladdr    = flag.String("waddelladdr", "", "if specified, connect to this waddell server and process NAT traversal requests inbound from waddell")
)

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
	if err == nil {
		go func() {
			// Read updates
			for {
				next := m.Next()
				nextCfg := next.(*Config)
				updateHandler(nextCfg)
			}
		}()
	}
	return initial.(*Config), err
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
		cfg.Country = "xx"
	}

	// Make sure we always have a Client config
	if cfg.Client == nil {
		cfg.Client = &client.ClientConfig{}
	}

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

// applyFlags updates this Config from any command-line flags that were passed
// in. ApplyFlags assumes that flag.Parse() has already been called.
func (updated *Config) applyFlags() error {
	if updated.Server == nil {
		updated.Server = &server.ServerConfig{}
	}

	// Visit all flags that have been set and copy to config
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		// General
		case "cloudconfig":
			updated.CloudConfig = *cloudconfig
		case "cloudconfigca":
			updated.CloudConfigCA = *cloudconfigca
		case "addr":
			updated.Addr = *addr
		case "role":
			updated.Role = *role
		case "instanceid":
			updated.InstanceId = *instanceid
		case "statsaddr":
			updated.StatsAddr = *statsaddr
		case "country":
			updated.Country = *country
		case "cpuprofile":
			updated.CpuProfile = *cpuprofile
		case "memprofile":
			updated.MemProfile = *memprofile

		// Server
		case "portmap":
			updated.Server.Portmap = *portmap
		case "server":
			updated.Server.AdvertisedHost = *advertisedHost
		case "waddelladdr":
			updated.Server.WaddellAddr = *waddelladdr
		}
	})

	return nil
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

var cloudflareMasquerades = []*client.Masquerade{
	&client.Masquerade{
		Domain: "minecraftforum.net",
		RootCA: "-----BEGIN CERTIFICATE-----\nMIIEYDCCA0igAwIBAgILBAAAAAABL07hRQwwDQYJKoZIhvcNAQEFBQAwVzELMAkG\nA1UEBhMCQkUxGTAXBgNVBAoTEEdsb2JhbFNpZ24gbnYtc2ExEDAOBgNVBAsTB1Jv\nb3QgQ0ExGzAZBgNVBAMTEkdsb2JhbFNpZ24gUm9vdCBDQTAeFw0xMTA0MTMxMDAw\nMDBaFw0yMjA0MTMxMDAwMDBaMF0xCzAJBgNVBAYTAkJFMRkwFwYDVQQKExBHbG9i\nYWxTaWduIG52LXNhMTMwMQYDVQQDEypHbG9iYWxTaWduIE9yZ2FuaXphdGlvbiBW\nYWxpZGF0aW9uIENBIC0gRzIwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIB\nAQDdNR3yIFQmGtDvpW+Bdllw3Of01AMkHyQOnSKf1Ccyeit87ovjYWI4F6+0S3qf\nZyEcLZVUunm6tsTyDSF0F2d04rFkCJlgePtnwkv3J41vNnbPMYzl8QbX3FcOW6zu\nzi2rqqlwLwKGyLHQCAeV6irs0Z7kNlw7pja1Q4ur944+ABv/hVlrYgGNguhKujiz\n4MP0bRmn6gXdhGfCZsckAnNate6kGdn8AM62pI3ffr1fsjqdhDFPyGMM5NgNUqN+\nARvUZ6UYKOsBp4I82Y4d5UcNuotZFKMfH0vq4idGhs6dOcRmQafiFSNrVkfB7cVT\n5NSAH2v6gEaYsgmmD5W+ZoiTAgMBAAGjggElMIIBITAOBgNVHQ8BAf8EBAMCAQYw\nEgYDVR0TAQH/BAgwBgEB/wIBADAdBgNVHQ4EFgQUXUayjcRLdBy77fVztjq3OI91\nnn4wRwYDVR0gBEAwPjA8BgRVHSAAMDQwMgYIKwYBBQUHAgEWJmh0dHBzOi8vd3d3\nLmdsb2JhbHNpZ24uY29tL3JlcG9zaXRvcnkvMDMGA1UdHwQsMCowKKAmoCSGImh0\ndHA6Ly9jcmwuZ2xvYmFsc2lnbi5uZXQvcm9vdC5jcmwwPQYIKwYBBQUHAQEEMTAv\nMC0GCCsGAQUFBzABhiFodHRwOi8vb2NzcC5nbG9iYWxzaWduLmNvbS9yb290cjEw\nHwYDVR0jBBgwFoAUYHtmGkUNl8qJUC99BM00qP/8/UswDQYJKoZIhvcNAQEFBQAD\nggEBABvgiADHBREc/6stSEJSzSBo53xBjcEnxSxZZ6CaNduzUKcbYumlO/q2IQen\nfPMOK25+Lk2TnLryhj5jiBDYW2FQEtuHrhm70t8ylgCoXtwtI7yw07VKoI5lkS/Z\n9oL2dLLffCbvGSuXL+Ch7rkXIkg/pfcNYNUNUUflWP63n41edTzGQfDPgVRJEcYX\npOBWYdw9P91nbHZF2krqrhqkYE/Ho9aqp9nNgSvBZnWygI/1h01fwlr1kMbawb30\nhag8IyrhFHvBN91i0ZJsumB9iOQct+R2UTjEqUdOqCsukNK1OFHrwZyKarXMsh3o\nwFZUTKiL8IkyhtyTMr5NGvo1dbU=\n-----END CERTIFICATE-----\n",
	},
}
