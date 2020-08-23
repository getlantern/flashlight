// This program is a simple config checker tool for proxies and global configs. It prints out basic
// statistics and round-trips the low-cardinality portions of the config in order to make sure the
// YAML parses as expected. The tool can either read a local config file (assumed to be plain text)
// or a remote http(s) URL (assumed to be gzipped).
//
// Examples:
//
//   go run main.go proxies ~/Library/Application\ Support/Lantern/proxies.yaml
//   go run main.go global ~/Library/Application\ Support/Lantern/global.yaml
//   go run main.go global https://globalconfig.flashlightproxy.com/global.yaml.gz
//
package main

import (
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/domainrouting"
	"github.com/getlantern/yaml"
)

func main() {
	if len(os.Args) < 3 {
		fail("Please specify a format ('proxies' or 'global') and a filename or http(s) url for the config to check")
	}

	format := os.Args[1]
	target := os.Args[2]

	u, err := url.Parse(target)
	if err != nil {
		fail("Unable to parse config url %v: %v", target, err)
	}

	var bytes []byte
	var readErr error

	switch u.Scheme {
	case "":
		bytes, readErr = ioutil.ReadFile(target)
	case "http", "https":
		bytes, readErr = readRemote(target)
	default:
		fail("Unrecognized url scheme: %v", u.Scheme)
	}

	if readErr != nil {
		fail("Unable to read %v: %v", target, readErr)
	}

	log("Checking %v config at %v", format, target)

	switch format {
	case "proxies":
		parseProxies(bytes)
	case "global":
		parseGlobal(bytes)
	default:
		fail("Unknown format %v, please specify either 'proxies' or 'global'", format)
	}
}

func readRemote(target string) ([]byte, error) {
	resp, err := http.Get(target)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Bad response status: %d", resp.StatusCode)
	}
	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(gzr)
}

func parseProxies(bytes []byte) {
	cfg := make(map[string]*chained.ChainedServerInfo)
	err := yaml.Unmarshal(bytes, cfg)
	if err != nil {
		fail("Unable to parse proxies config: %v", err)
	}
	log("Number of proxies: %d", len(cfg))
	out, err := yaml.Marshal(cfg)
	if err != nil {
		fail("Unable to marshal proxies config")
	}

	log("------ Round-tripped YAML ------")
	os.Stdout.Write(out)
}

func parseGlobal(bytes []byte) {
	cfg := &config.Global{}
	err := yaml.Unmarshal(bytes, cfg)
	if err != nil {
		fail("Unable to parse global config: %v", err)
	}

	log("Number of Proxied Sites: %d", len(cfg.ProxiedSites.Cloud))

	var direct, proxied int
	for _, rule := range cfg.DomainRoutingRules {
		switch rule {
		case domainrouting.Direct:
			direct++
		case domainrouting.Proxy:
			proxied++
		}
	}
	log("Domainrouting direct: %d", direct)
	log("Domainrouting proxied: %d", proxied)

	for name, provider := range cfg.Client.FrontedProviders() {
		log("Masquerades for %v: %d", name, len(provider.Masquerades))
	}

	// Clear out high cardinality data before marshaling
	cfg.ProxiedSites = nil
	cfg.DomainRoutingRules = nil
	cfg.Client.MasqueradeSets = nil
	cfg.Client.Fronted.Providers = nil

	out, err := yaml.Marshal(cfg)
	if err != nil {
		fail("Unable to marshal global config")
	}

	log("------ Round-tripped YAML ------")
	os.Stdout.Write(out)
}

func log(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
}

func fail(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}
