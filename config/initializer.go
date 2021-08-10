package config

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config/generated"
)

var (
	log = golog.LoggerFor("flashlight.config")

	// DefaultProxyConfigPollInterval determines how frequently to fetch proxies.yaml
	DefaultProxyConfigPollInterval = 1 * time.Minute

	// ForceProxyConfigPollInterval overrides how frequently to fetch proxies.yaml if set (does not honor values from global.yaml)
	ForceProxyConfigPollInterval = 0 * time.Second

	// DefaultGlobalConfigPollInterval determines how frequently to fetch global.yaml
	DefaultGlobalConfigPollInterval = 1 * time.Hour
)

// Init determines the URLs at which to fetch proxy and global config and
// passes those to InitWithURLs, which initializes the config setup for both
// fetching per-user proxies as well as the global config. It returns a function
// that can be used to stop the reading of configs.
func Init(configDir string, flags map[string]interface{},
	userConfig common.UserConfig, proxiesDispatch func(interface{}, Source),
	origGlobalDispatch func(interface{}, Source), rt http.RoundTripper) (stop func()) {
	staging := isStaging(flags)
	proxyConfigURL := checkOverrides(flags, getProxyURL(staging), "proxies.yaml.gz")
	globalConfigURL := checkOverrides(flags, getGlobalURL(staging), "global.yaml.gz")

	return InitWithURLs(configDir, flags, userConfig, proxiesDispatch,
		origGlobalDispatch, proxyConfigURL, globalConfigURL, rt)
}

type cfgWithSource struct {
	cfg interface{}
	src Source
}

// InitWithURLs initializes the config setup for both fetching per-user proxies
// as well as the global config given a set of URLs for fetching proxy and
// global config. It returns a function that can be used to stop the reading of
// configs.
func InitWithURLs(configDir string, flags map[string]interface{},
	userConfig common.UserConfig, origProxiesDispatch func(interface{}, Source),
	origGlobalDispatch func(interface{}, Source), proxyURL string,
	globalURL string, rt http.RoundTripper) (stop func()) {
	var mx sync.RWMutex
	globalConfigPollInterval := DefaultGlobalConfigPollInterval
	proxyConfigPollInterval := DefaultProxyConfigPollInterval
	if ForceProxyConfigPollInterval > 0 {
		proxyConfigPollInterval = ForceProxyConfigPollInterval
	}

	globalDispatchCh := make(chan cfgWithSource)
	proxiesDispatchCh := make(chan cfgWithSource)
	go func() {
		for c := range globalDispatchCh {
			origGlobalDispatch(c.cfg, c.src)
		}
	}()
	go func() {
		for c := range proxiesDispatchCh {
			origProxiesDispatch(c.cfg, c.src)
		}
	}()

	globalDispatch := func(cfg interface{}, src Source) {
		globalConfig, ok := cfg.(*Global)
		if ok {
			mx.Lock()
			if globalConfig.GlobalConfigPollInterval > 0 {
				globalConfigPollInterval = globalConfig.GlobalConfigPollInterval
			}
			if ForceProxyConfigPollInterval == 0 && globalConfig.ProxyConfigPollInterval > 0 {
				proxyConfigPollInterval = globalConfig.ProxyConfigPollInterval
			}
			mx.Unlock()
		}
		// Rather than call `origGlobalDispatch` here, we are calling it in a
		// separate goroutine (initiated above) that listens for messages on
		// `globalDispatchCh`. This (a) avoids blocking Lantern startup when
		// applying new configuration and (b) allows us to serialize application of
		// config changes.
		globalDispatchCh <- cfgWithSource{cfg, src}
	}

	proxiesDispatch := func(cfg interface{}, src Source) {
		proxiesDispatchCh <- cfgWithSource{cfg, src}
	}

	// These are the options for fetching the per-user proxy config.
	proxyOptions := &options{
		saveDir:                 configDir,
		obfuscate:               obfuscate(flags),
		name:                    "proxies.yaml",
		originURL:               proxyURL,
		userConfig:              userConfig,
		unmarshaler:             newProxiesUnmarshaler(),
		dispatch:                proxiesDispatch,
		embeddedData:            generated.EmbeddedProxies,
		forceUseEmbeddedConfigs: shouldForceUseEmbeddedProxies(flags),
		sleep: func() time.Duration {
			mx.RLock()
			defer mx.RUnlock()
			return proxyConfigPollInterval
		},
		sticky: isSticky(flags),
		rt:     rt,
	}

	stopProxies := pipeConfig(proxyOptions)

	// These are the options for fetching the global config.
	globalOptions := &options{
		saveDir:                 configDir,
		obfuscate:               obfuscate(flags),
		name:                    "global.yaml",
		originURL:               globalURL,
		userConfig:              userConfig,
		unmarshaler:             newGlobalUnmarshaler(flags),
		dispatch:                globalDispatch,
		embeddedData:            generated.GlobalConfig,
		forceUseEmbeddedConfigs: shouldForceUseEmbeddedGlobalConfig(flags),
		sleep: func() time.Duration {
			mx.RLock()
			defer mx.RUnlock()
			return globalConfigPollInterval
		},
		sticky: isSticky(flags),
		rt:     rt,
	}

	stopGlobal := pipeConfig(globalOptions)

	return func() {
		log.Debug("*************** Stopping Config")
		stopProxies()
		stopGlobal()
	}
}

func newGlobalUnmarshaler(flags map[string]interface{}) func(bytes []byte) (interface{}, error) {
	return func(bytes []byte) (interface{}, error) {
		gl := NewGlobal()
		gl.applyFlags(flags)
		if err := yaml.Unmarshal(bytes, gl); err != nil {
			return nil, err
		}
		if err := gl.validate(); err != nil {
			return nil, err
		}
		return gl, nil
	}
}

func newProxiesUnmarshaler() func(bytes []byte) (interface{}, error) {
	return func(bytes []byte) (interface{}, error) {
		servers := make(map[string]*chained.ChainedServerInfo)
		if err := yaml.Unmarshal(bytes, servers); err != nil {
			return nil, err
		}
		if len(servers) == 0 {
			return nil, errors.New("No chained server")
		}
		return servers, nil
	}
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

func shouldForceUseEmbeddedGlobalConfig(flags map[string]interface{}) bool {
	return checkBool(flags, "force-use-embedded-globalconfig")
}

func shouldForceUseEmbeddedProxies(flags map[string]interface{}) bool {
	return checkBool(flags, "force-use-embedded-proxies")
}

func checkBool(flags map[string]interface{}, key string) bool {
	if s, ok := flags[key].(bool); ok {
		return s
	}
	return false
}

func checkOverrides(flags map[string]interface{},
	url string, name string) string {
	if s, ok := flags["cloudconfig"].(string); ok {
		if len(s) > 0 {
			log.Debugf("Overridding config URL from the command line '%v'", s)
			return s + "/" + name
		}
	}
	return url
}

// getProxyURL returns the proxy URL to use depending on whether or not
// we're in staging.
func getProxyURL(staging bool) string {
	if staging {
		log.Debug("Will obtain proxies.yaml from staging service")
		return common.ProxiesStagingURL
	}
	log.Debug("Will obtain proxies.yaml from production service")
	return common.ProxiesURL
}

// getGlobalURL returns the global URL to use depending on whether or not
// we're in staging.
func getGlobalURL(staging bool) string {
	if staging {
		log.Debug("Will obtain global.yaml from staging service")
		return common.GlobalStagingURL
	}
	log.Debugf("Will obtain global.yaml from production service at %v", common.GlobalURL)
	return common.GlobalURL
}
