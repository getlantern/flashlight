package main

import (
	"flag"
	"time"
)

var (
	addr                  = flag.String("addr", "", "ip:port on which to listen for requests. When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https (required)")
	socksaddr             = flag.String("socksaddr", "", "ip:port on which to listen for SOCKS5 proxy requests.")
	configdir             = flag.String("configdir", "", "directory in which to store configuration. Defaults to platform-specific directories.")
	cloudconfig           = flag.String("cloudconfig", "", "optional http(s) URL to a cloud-based source for configuration updates")
	cloudconfigca         = flag.String("cloudconfigca", "", "optional PEM encoded certificate used to verify TLS connections to fetch cloudconfig")
	registerat            = flag.String("registerat", "", "base URL for peer DNS registry at which to register (e.g. https://peerscanner.getiantem.org)")
	country               = flag.String("country", "xx", "2 digit country code under which to report stats. Defaults to xx.")
	cpuprofile            = flag.String("cpuprofile", "", "write cpu profile to given file")
	memprofile            = flag.String("memprofile", "", "write heap profile to given file")
	uiaddr                = flag.String("uiaddr", "", "if specified, indicates host:port the UI HTTP server should be started on")
	proxyAll              = flag.Bool("proxyall", false, "set to true to proxy all traffic through Lantern network")
	stickyConfig          = flag.Bool("stickyconfig", false, "set to true to only use the local config file")
	headless              = flag.Bool("headless", false, "if true, lantern will run with no ui")
	startup               = flag.Bool("startup", false, "if true, Lantern was automatically run on system startup")
	clearProxySettings    = flag.Bool("clear-proxy-settings", false, "if true, Lantern removes proxy settings from the system.")
	pprofAddr             = flag.String("pprofaddr", "", "pprof address to listen on, not activate pprof if empty")
	forceProxyAddr        = flag.String("force-proxy-addr", "", "if specified, force chained proxying to use this address instead of the configured one, assuming an HTTP proxy")
	forceAuthToken        = flag.String("force-auth-token", "", "if specified, force chained proxying to use this auth token instead of the configured one")
	forceConfigCountry    = flag.String("force-config-country", "", "if specified, force config fetches to pretend they're coming from this 2 letter country-code")
	readableconfig        = flag.Bool("readableconfig", false, "if specified, disables obfuscation of the config yaml so that it remains human readable")
	bordaReportInterval   = flag.Duration("borda-report-interval", 15*time.Minute, "How frequently to report errors to borda. Set to 0 to disable reporting.")
	bordaSamplePercentage = flag.Float64("borda-sample-percentage", 0.01, "The percentage of activity to report to Borda (0.01 = 1%)")
	help                  = flag.Bool("help", false, "Get usage help")
)

// flagsAsMap returns a map of all flags that were provided at runtime
func flagsAsMap() map[string]interface{} {
	flags := make(map[string]interface{})
	flag.VisitAll(func(f *flag.Flag) {
		switch fl := f.Value.(type) {
		case flag.Getter:
			flags[f.Name] = fl.Get()
		default:
			log.Debugf("Received unexpected flag: %v", f)
		}
	})
	// Some properties should always be included
	flags["cpuprofile"] = *cpuprofile
	flags["memprofile"] = *memprofile

	return flags
}
