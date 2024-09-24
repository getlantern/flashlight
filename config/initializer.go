package config

import (
	"errors"
	"net/http"
	"sync"
	"time"

	commonconfig "github.com/getlantern/common/config"
	"github.com/getlantern/golog"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/embeddedconfig"
)

const packageLogPrefix = "flashlight.config"

var (
	log = golog.LoggerFor(packageLogPrefix)

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
func Init(
	configDir string, flags map[string]interface{}, userConfig common.UserConfig,
	proxiesDispatch func(interface{}, Source), onProxiesSaveError func(error),
	origGlobalDispatch func(interface{}, Source), onGlobalSaveError func(error),
	rt http.RoundTripper) (stop func()) {

	proxyConfigURL := checkOverrides(flags, common.ProxiesURL, "proxies.yaml.gz")
	globalConfigURL := checkOverrides(flags, common.GlobalURL, "global.yaml.gz")

	return InitWithURLs(
		configDir, flags, userConfig, proxiesDispatch, onProxiesSaveError,
		origGlobalDispatch, onGlobalSaveError, proxyConfigURL, globalConfigURL, rt)
}

type cfgWithSource struct {
	cfg interface{}
	src Source
}

// InitWithURLs initializes the config setup for both fetching per-user proxies
// as well as the global config given a set of URLs for fetching proxy and
// global config. It returns a function that can be used to stop the reading of
// configs.
func InitWithURLs(
	configDir string, flags map[string]interface{}, userConfig common.UserConfig,
	origProxiesDispatch func(interface{}, Source), onProxiesSaveError func(error),
	origGlobalDispatch func(interface{}, Source), onGlobalSaveError func(error),
	proxyURL string, globalURL string, rt http.RoundTripper) (stop func()) {

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
		saveDir:          configDir,
		onSaveError:      onProxiesSaveError,
		obfuscate:        obfuscate(flags),
		name:             "proxies.yaml",
		originURL:        proxyURL,
		userConfig:       userConfig,
		unmarshaler:      newProxiesUnmarshaler(),
		dispatch:         proxiesDispatch,
		embeddedData:     embeddedconfig.Proxies,
		embeddedRequired: false,
		sleep: func() time.Duration {
			mx.RLock()
			defer mx.RUnlock()
			return proxyConfigPollInterval
		},
		sticky: isSticky(flags),
		rt:     rt,
		// Proxies are not provided over the DHT (yet! ᕕ( ᐛ )ᕗ), so dhtupContext is not passed.
	}

	stopProxies := pipeConfig(proxyOptions)

	// These are the options for fetching the global config.
	globalOptions := &options{
		saveDir:          configDir,
		onSaveError:      onGlobalSaveError,
		obfuscate:        obfuscate(flags),
		name:             "global.yaml",
		originURL:        globalURL,
		userConfig:       userConfig,
		unmarshaler:      newGlobalUnmarshaler(flags),
		dispatch:         globalDispatch,
		embeddedData:     embeddedconfig.Global,
		embeddedRequired: true,
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
		servers := make(map[string]*commonconfig.ProxyConfig)
		if err := yaml.Unmarshal(bytes, servers); err != nil {
			return nil, err
		}
		if len(servers) == 0 {
			return nil, errors.New("no chained server")
		}
		return servers, nil
	}
}

func obfuscate(flags map[string]interface{}) bool {
	return flags["readableconfig"] == nil || !flags["readableconfig"].(bool)
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
	url string, name string) string {
	if s, ok := flags["cloudconfig"].(string); ok {
		if len(s) > 0 {
			log.Debugf("Overridding config URL from the command line '%v'", s)
			return s + "/" + name
		}
	}
	return url
}
