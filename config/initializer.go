package config

import (
	"errors"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config/generated"
)

var (
	log = golog.LoggerFor("flashlight.config")

	// globalURLs are the chained and fronted URLs for fetching the global config.
	globalURLs = &chainedFrontedURLs{
		chained: "https://globalconfig.flashlightproxy.com/global.yaml.gz",
		fronted: "https://d24ykmup0867cj.cloudfront.net/global.yaml.gz",
	}

	// globalStagingURLs are the chained and fronted URLs for fetching the global
	// config in a staging environment.
	globalStagingURLs = &chainedFrontedURLs{
		chained: "https://globalconfig.flashlightproxy.com/global.yaml.gz",
		fronted: "https://d24ykmup0867cj.cloudfront.net/global.yaml.gz",
	}

	// The following are over HTTP because proxies do not forward X-Forwarded-For
	// with HTTPS and because we only support falling back to direct domain
	// fronting through the local proxy for HTTP.

	// proxiesURLs are the chained and fronted URLs for fetching the per user
	// proxy config.
	proxiesURLs = &chainedFrontedURLs{
		chained: "http://config.getiantem.org/proxies.yaml.gz",
		fronted: "http://d2wi0vwulmtn99.cloudfront.net/proxies.yaml.gz",
	}

	// proxiesStagingURLs are the chained and fronted URLs for fetching the per user
	// proxy config in a staging environment.
	proxiesStagingURLs = &chainedFrontedURLs{
		chained: "http://config-staging.getiantem.org/proxies.yaml.gz",
		fronted: "http://d33pfmbpauhmvd.cloudfront.net/proxies.yaml.gz",
	}

	// ProxyConfigPollInterval determines how frequently to fetch proxies.yaml
	ProxyConfigPollInterval = 1 * time.Minute
)

// Init initializes the config setup for both fetching per-user proxies as well
// as the global config.
func Init(configDir string, flags map[string]interface{},
	authConfig common.AuthConfig, proxiesDispatch func(interface{}),
	globalDispatch func(interface{})) {
	staging := isStaging(flags)
	// These are the options for fetching the per-user proxy config.
	proxyOptions := &options{
		saveDir:    configDir,
		obfuscate:  obfuscate(flags),
		name:       "proxies.yaml",
		urls:       checkOverrides(flags, getProxyURLs(staging), "proxies.yaml.gz"),
		authConfig: authConfig,
		unmarshaler: func(bytes []byte) (interface{}, error) {
			servers := make(map[string]*chained.ChainedServerInfo)
			if err := yaml.Unmarshal(bytes, servers); err != nil {
				return nil, err
			}
			if len(servers) == 0 {
				return nil, errors.New("No chained server")
			}
			return servers, nil
		},
		dispatch:     proxiesDispatch,
		embeddedData: generated.EmbeddedProxies,
		sleep:        ProxyConfigPollInterval,
		sticky:       isSticky(flags),
	}

	pipeConfig(proxyOptions)

	// These are the options for fetching the global config.
	globalOptions := &options{
		saveDir:    configDir,
		obfuscate:  obfuscate(flags),
		name:       "global.yaml",
		urls:       checkOverrides(flags, getGlobalURLs(staging), "global.yaml.gz"),
		authConfig: authConfig,
		unmarshaler: func(bytes []byte) (interface{}, error) {
			gl := newGlobal()
			gl.applyFlags(flags)
			if err := yaml.Unmarshal(bytes, gl); err != nil {
				return nil, err
			}
			if err := gl.validate(); err != nil {
				return nil, err
			}
			return gl, nil
		},
		dispatch:     globalDispatch,
		embeddedData: generated.GlobalConfig,
		sleep:        24 * time.Hour,
		sticky:       false,
	}

	pipeConfig(globalOptions)
}

func obfuscate(flags map[string]interface{}) bool {
	return flags["readableconfig"] == nil || !flags["readableconfig"].(bool)
}

func isStaging(flags map[string]interface{}) bool {
	return checkBool(flags, "staging")
}

func isSticky(flags map[string]interface{}) bool {
	return checkBool(flags, "stickyconfig")
}

func checkBool(flags map[string]interface{}, key string) bool {
	if s, ok := flags[key].(bool); ok {
		return s
	}
	return false
}

func checkOverrides(flags map[string]interface{},
	urls *chainedFrontedURLs, name string) *chainedFrontedURLs {
	if s, ok := flags["cloudconfig"].(string); ok {
		if len(s) > 0 {
			log.Debugf("Overridding chained URL from the command line '%v'", s)
			urls.chained = s + "/" + name
		}
	}
	if s, ok := flags["frontedconfig"].(string); ok {
		if len(s) > 0 {
			log.Debugf("Overridding fronted URL from the command line '%v'", s)
			urls.fronted = s + "/" + name
		}
	}
	return urls
}

// getProxyURLs returns the proxy URLs to use depending on whether or not
// we're in staging.
func getProxyURLs(staging bool) *chainedFrontedURLs {
	if staging {
		log.Debug("Configuring for staging")
		return proxiesStagingURLs
	}
	log.Debugf("Not configuring for staging.")
	return proxiesURLs
}

// getGlobalURLs returns the global URLs to use depending on whether or not
// we're in staging.
func getGlobalURLs(staging bool) *chainedFrontedURLs {
	if staging {
		log.Debug("Configuring for staging")
		return globalStagingURLs
	}
	log.Debugf("Not configuring for staging.")
	return globalURLs
}
