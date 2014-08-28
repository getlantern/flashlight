package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/liveyaml"
	"github.com/getlantern/yaml"
)

type Config struct {
	ConfigDir      string
	CloudConfig    string
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
					MasqueradeAs: "elance.com",
					RootCA:       "-----BEGIN CERTIFICATE-----\nMIIERjCCAy6gAwIBAgIEByd1ijANBgkqhkiG9w0BAQUFADBaMQswCQYDVQQGEwJJ\nRTESMBAGA1UEChMJQmFsdGltb3JlMRMwEQYDVQQLEwpDeWJlclRydXN0MSIwIAYD\nVQQDExlCYWx0aW1vcmUgQ3liZXJUcnVzdCBSb290MB4XDTEyMDcyNTE3NTgyOFoX\nDTE5MDcyNTE3NTc0NFowbDELMAkGA1UEBhMCVVMxFTATBgNVBAoTDERpZ2lDZXJ0\nIEluYzEZMBcGA1UECxMQd3d3LmRpZ2ljZXJ0LmNvbTErMCkGA1UEAxMiRGlnaUNl\ncnQgSGlnaCBBc3N1cmFuY2UgRVYgUm9vdCBDQTCCASIwDQYJKoZIhvcNAQEBBQAD\nggEPADCCAQoCggEBAMbM5XPm+9S75S0tMqbf5YE/yc0lSbZxKsPVlDRnogocsF9p\npkCxxLeyj9CYpKlBWTrT3JTWPNt0OKRKzE0lgvdKpVMSOO7zSW1xkX5jtqumX8Ok\nhPhPYlG++MXs2ziS4wblCJEMxChBVfvLWokVfnHoNb9Ncgk9vjo4UFt3MRuNs8ck\nRZqnrG0AFFoEt7oT61EKmEFBIk5lYYeBQVCmeVyJ3hlKV9Uu5l0cUyx+mM0aBhak\naHPQNAQTXKFx01p8VdteZOE3hzBWBOURtCmAEvF5OYiiAhF8J2a3iLd48soKqDir\nCmTCv2ZdlYTBoSUeh10aUAsgEsxBu24LUTi4S8sCAwEAAaOCAQAwgf0wEgYDVR0T\nAQH/BAgwBgEB/wIBATBTBgNVHSAETDBKMEgGCSsGAQQBsT4BADA7MDkGCCsGAQUF\nBwIBFi1odHRwOi8vY3liZXJ0cnVzdC5vbW5pcm9vdC5jb20vcmVwb3NpdG9yeS5j\nZm0wDgYDVR0PAQH/BAQDAgEGMB8GA1UdIwQYMBaAFOWdWTCCR1jMrPoIVDaGezq1\nBE3wMEIGA1UdHwQ7MDkwN6A1oDOGMWh0dHA6Ly9jZHAxLnB1YmxpYy10cnVzdC5j\nb20vQ1JML09tbmlyb290MjAyNS5jcmwwHQYDVR0OBBYEFLE+w2kD+L9HAdSYJhoI\nAu9jZCvDMA0GCSqGSIb3DQEBBQUAA4IBAQB2Vlg2DRmYtNmlyzB1rrHWgJfM7jhy\naDmwAj5GtsTyrNHS4WYW5oWkVXfLLhxZ3aVL3y8zu85gVyc6oU1Jb1V2bdXXwqBb\nKpv5S/d/Id3uXFcNADU68YxGywT2Ro/OBWrVxGz+bpi/pJy9joksvnEBQ8w2KmQG\nVpeTpUe9Sj+MG3XInrDwJZh3IcB2p1F6JCV9GDUG/sEJxQ47majNnSmwOon16ucq\n5eIkTmipHafd0ghLodFvDL0s4Lt8+qE8Zc86UkvTIHoKEFX4rUMWVCdOU3PIo5aJ\n0OF5xgl41fW9sbPFf6ZLr0kRyJecT3xwaRZcLbjQ3xwyUrne88MG6IMi\n-----END CERTIFICATE-----\n",
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
	flag.StringVar(&cfg.CloudConfig, "cloudconfig", cfg.CloudConfig, "optional http(s) URL to a cloud-based source for configuration updates")
	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "ip:port on which to listen for requests. When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https (required)")
	flag.IntVar(&cfg.Portmap, "portmap", cfg.Portmap, "try to map this port on the firewall to the port on which flashlight is listening, using UPnP or NAT-PMP. If mapping this port fails, flashlight will exit with status code 50")
	flag.StringVar(&cfg.Role, "role", cfg.Role, "either 'client' or 'server' (required)")
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

// Merges the newer config into this config, which involves replacing the
// client's list of servers with the value from the newer config.  The merged
// config is saved to disk.
func (cfg *Config) Merge(newer []byte) error {
	merged := &Config{}
	err := liveyaml.Load(cfg.configFile(), merged)
	if err != nil {
		return fmt.Errorf("Unable to load config from %s: %s", cfg.configFile(), err)
	}
	err = yaml.Unmarshal(newer, merged)
	if err != nil {
		return err
	}
	return merged.Save()
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
