package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/liveyaml"
)

type Config struct {
	ConfigDir      string
	Addr           string
	Portmap        int
	Role           string
	Client         *client.ClientConfig
	AdvertisedHost string
	InstanceId     string
	StatsAddr      string
	Country        string
	DumpHeaders    bool
	CpuProfile     string
	MemProfile     string
}

func DefaultConfig() *Config {
	return &Config{
		ConfigDir: ".",
		Client: &client.ClientConfig{
			Servers: map[string]*client.ServerInfo{
				"roundrobin": &client.ServerInfo{
					Host:         "roundrobin.getiantem.org",
					Port:         443,
					MasqueradeAs: "cdnjs.com",
					QOS:          10,
					Weight:       1000000,
				},
			},
		},
		Country: "xx",
	}
}

func (cfg *Config) IsDownstream() bool {
	return cfg.Role == "client"
}

func (cfg *Config) IsUpstream() bool {
	return !cfg.IsDownstream()
}

func (cfg *Config) InitFlags() {
	flag.StringVar(&cfg.ConfigDir, "configdir", cfg.ConfigDir, "directory in which to store configuration, including flashlight.yaml (defaults to current directory)")
	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "ip:port on which to listen for requests. When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https (required)")
	flag.IntVar(&cfg.Portmap, "portmap", cfg.Portmap, "try to map this port on the firewall to the port on which flashlight is listening, using UPnP or NAT-PMP. If mapping this port fails, flashlight will exit with status code 50")
	flag.StringVar(&cfg.Role, "role", cfg.Role, "either 'client' or 'server' (required)")
	flag.StringVar(&cfg.Client.Servers["roundrobin"].Host, "host", cfg.Client.Servers["roundrobin"].Host, "Hostname of upstream server")
	flag.StringVar(&cfg.Client.Servers["roundrobin"].MasqueradeAs, "masquerade", cfg.Client.Servers["roundrobin"].MasqueradeAs, "masquerade host: if specified, flashlight will actually make a request to this host's IP but with a host header corresponding to the 'server' parameter")
	flag.StringVar(&cfg.Client.Servers["roundrobin"].RootCA, "rootca", cfg.Client.Servers["roundrobin"].RootCA, "pin to this CA cert if specified (PEM format)")
	flag.StringVar(&cfg.AdvertisedHost, "server", cfg.AdvertisedHost, "FQDN of flashlight server when running in server mode (required)")
	flag.StringVar(&cfg.InstanceId, "instanceid", cfg.InstanceId, "instanceId under which to report stats to statshub. If not specified, no stats are reported.")
	flag.StringVar(&cfg.StatsAddr, "statsaddr", cfg.StatsAddr, "host:port at which to make detailed stats available using server-sent events (optional)")
	flag.StringVar(&cfg.Country, "country", cfg.Country, "2 digit country code under which to report stats. Defaults to xx.")
	flag.BoolVar(&cfg.DumpHeaders, "dumpheaders", cfg.DumpHeaders, "dump the headers of outgoing requests and responses to stdout")
	flag.StringVar(&cfg.CpuProfile, "cpuprofile", cfg.CpuProfile, "write cpu profile to given file")
	flag.StringVar(&cfg.MemProfile, "memprofile", cfg.MemProfile, "write heap profile to given file")
}

func (cfg *Config) Bind(updates chan *Config, errors chan error) error {
	cf := cfg.configFile()
	untypedUpdates := make(chan interface{}, len(updates))
	err := liveyaml.Bind(cf, cfg, untypedUpdates, errors)
	if err != nil {
		return fmt.Errorf("Unable to bind config to yaml file %s: %s", cf, err)
	}

	// Convert channel types
	go func() {
		for {
			update := <-untypedUpdates
			updates <- update.(*Config)
		}
	}()

	return nil
}

func (cfg *Config) Save() error {
	return liveyaml.Save(cfg.configFile(), cfg)
}

// InConfigDir returns the path to the given filename inside of the ConfigDir.
func (cfg *Config) InConfigDir(filename string) string {
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

func (cfg *Config) configFile() string {
	return cfg.InConfigDir("flashlight.yaml")
}
