package flashlight

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/jaffee/commandeer"
	"github.com/mitchellh/mapstructure"
)

type Flags struct {
	ReplicaDevConfig bool   `flag:"replica-dev-config" help:"use the hard-coded replica global config"`
	Addr                         string        `flag:"addr" help:"ip:port on which to listen for requests. When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https"`
	SocksAddr                    string        `flag:"socksaddr" help:"ip:port on which to listen for SOCKS5 proxy requests."`
	ConfigDir                    string        `flag:"configdir" help:"directory in which to store configuration. Defaults to platform-specific directories."`
	VPN                          bool          `help:"specify this flag to enable vpn mode"`
	CloudConfig                  string        `flag:"cloudconfig" help:"optional http(s) URL to a cloud-based source for configuration updates"`
	CloudConfigCA                string        `flag:"cloudconfigca" help:"optional PEM encoded certificate used to verify TLS connections to fetch cloudconfig"`
	RegisterAt                   string        `flag:"registerat" help:"base URL for peer DNS registry at which to register (e.g. https://peerscanner.getiantem.org)"`
	ClearProxySettings           bool          `flag:"clear-proxy-settings" help:"if true, Lantern removes proxy settings from the system."`
	CpuProfile                   string        `flag:"cpuprofile" help:"write cpu profile to given file"`
	MemProfile                   string        `flag:"memprofile" help:"write heap profile to given file"`
	UIAddr                       string        `flag:"uiaddr"  help:"if specified, indicates host:port the UI HTTP server should be started on"`
	ProxyAll                     bool          `flag:"proxyall"  help:"set to true to proxy all traffic through Lantern network"`
	StickyConfig                 bool          `flag:"stickyconfig" help:"set to true to only use the local config file"`
	ForceUseEmbeddedGlobalConfig bool          `flag:"force-use-embedded-globalconfig" help:"set to true to force the usage of the hard-coded global config in config/generated/embeddedGlobal.go".`
	ForceUseEmbeddedProxies      bool          `flag:"force-use-embedded-proxies" help:"set to true to force the use the hard-coded proxies in config/generated/embeddedProxies.go".`
	AuthAddr                     string        `flag:"authaddr" help:"if specified, indicates the address to use for the Lantern auth server"`
	YinbiAddr                    string        `flag:"yinbiaddr" help:"if specified, indicates the address to use for the Yinbi server"`
	Headless                     bool          `help:"if true, lantern will run with no ui"`
	Startup                      bool          `help:"if true, Lantern was automatically run on system startup"`
	PprofAddr                    string        `flag:"pprofaddr" help:"pprof address to listen on, not activate pprof if empty"`
	ForceProxyAddr               string        `flag:"force-proxy-addr" help:"if specified, force chained proxying to use this address instead of the configured one, assuming an HTTP proxy"`
	ForceAuthToken               string        `flag:"force-auth-token" help:"if specified, force chained proxying to use this auth token instead of the configured one"`
	ForceConfigCountry           string        `flag:"force-config-country" help:"if specified, force config fetches to pretend they're coming from this 2 letter country-code"`
	ReadableConfig               bool          `flag:"readableconfig" help:"if specified, disables obfuscation of the config yaml so that it remains human readable"`
	Help                         bool          `flag:"help" help:"Get usage help"`
	NoUiHttpToken                bool          `flag:"no-ui-http-token" help:"don't require a HTTP token from the UI"`
	Standalone                   bool          `flag:"standalone" help:"run Lantern in its own browser window (doesn't rely on system browser)"`
	Initialize                   bool          `flag:"initialize" help:"silently initialize Lantern to a state of having working proxy and exit, typically during installation."`
	Timeout                      time.Duration `flag:"timeout" help:"force stop Lantern with an exit status of -1 after the timeout."`
	ReplicaRustUrl               string        `flag:"replica-rust-url" help:"If specified, use the replica-rust service at the provided endpoint. Else, use the per-country endpoints in global config"`
	Staging                      bool          `flag:"-"`
}

func (f Flags) AsMap() map[string]interface{} {
	var result map[string]interface{}
	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "flag",
		Result:  &result,
	})
	_ = decoder.Decode(&f)
	return result
}

type flagSet struct {
	*flag.FlagSet
}

func (f *flagSet) Flags() (flags []string) {
	f.VisitAll(func(f *flag.Flag) {
		flags = append(flags, f.Name)
	})
	return flags
}

var _ = commandeer.FlagNamer(&flagSet{})

func ParseFlags() Flags {
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

	// here we can define default values
	cfg := Flags{
		Country:        "xx",
		ReplicaRustUrl: "https://replica-search.lantern.io",
	}

	// the following will error on invalid arguments and take env variables starting with LANTERN_ into consideration
	err := commandeer.LoadArgsEnv(&flagSet{flag.CommandLine}, &cfg, args, "LANTERN_", nil)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.Help {
		flag.Usage()
		os.Exit(1)
	}
	return cfg

}
