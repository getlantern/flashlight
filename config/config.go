package config

import (
	"flag"
	"fmt"
	"os"

	"github.com/getlantern/liveyaml"
)

type Config struct {
	ConfigDir    string
	Addr         string
	Portmap      int
	Role         string
	UpstreamHost string
	UpstreamPort int
	MasqueradeAs string
	RootCA       string
	InstanceId   string
	StatsAddr    string
	Country      string
	DumpHeaders  bool
	CpuProfile   string
	MemProfile   string
	ParentPID    int
}

func Default() *Config {
	return &Config{
		ConfigDir:    ".",
		UpstreamPort: 443,
		Country:      "xx",
	}
}

func (c *Config) IsDownstream() bool {
	return c.Role == "client"
}

func (c *Config) IsUpstream() bool {
	return !c.IsDownstream()
}

func (c *Config) InitFlags() {
	flag.StringVar(&c.ConfigDir, "configdir", c.ConfigDir, "directory in which to store configuration, including flashlight.yaml (defaults to current directory)")
	flag.StringVar(&c.Addr, "addr", c.Addr, "ip:port on which to listen for requests. When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https (required)")
	flag.IntVar(&c.Portmap, "portmap", c.Portmap, "try to map this port on the firewall to the port on which flashlight is listening, using UPnP or NAT-PMP. If mapping this port fails, flashlight will exit with status code 50")
	flag.StringVar(&c.Role, "role", c.Role, "either 'client' or 'server' (required)")
	flag.StringVar(&c.UpstreamHost, "server", c.UpstreamHost, "FQDN of flashlight server (required)")
	flag.IntVar(&c.UpstreamPort, "serverport", c.UpstreamPort, "the port on which to connect to the server")
	flag.StringVar(&c.MasqueradeAs, "masquerade", c.MasqueradeAs, "masquerade host: if specified, flashlight will actually make a request to this host's IP but with a host header corresponding to the 'server' parameter")
	flag.StringVar(&c.RootCA, "rootca", c.RootCA, "pin to this CA cert if specified (PEM format)")
	flag.StringVar(&c.InstanceId, "instanceid", c.InstanceId, "instanceId under which to report stats to statshub. If not specified, no stats are reported.")
	flag.StringVar(&c.StatsAddr, "statsaddr", c.StatsAddr, "host:port at which to make detailed stats available using server-sent events (optional)")
	flag.StringVar(&c.Country, "country", c.Country, "2 digit country code under which to report stats. Defaults to xx.")
	flag.BoolVar(&c.DumpHeaders, "dumpheaders", c.DumpHeaders, "dump the headers of outgoing requests and responses to stdout")
	flag.StringVar(&c.CpuProfile, "cpuprofile", c.CpuProfile, "write cpu profile to given file")
	flag.StringVar(&c.MemProfile, "memprofile", c.MemProfile, "write heap profile to given file")
	flag.IntVar(&c.ParentPID, "parentpid", c.ParentPID, "the parent process's PID, used on Windows for killing flashlight when the parent disappears")
}

func (c *Config) Bind() error {
	configFile := fmt.Sprintf("%s%cflashlight.yaml", c.ConfigDir, os.PathSeparator)
	err := liveyaml.Bind(configFile, c)
	if err != nil {
		return fmt.Errorf("Unable to bind config to yaml file %s: %s", configFile, err)
	}
	return nil
}
