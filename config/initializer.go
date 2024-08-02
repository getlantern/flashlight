package config

import (
	"net/http"
	"sync"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/embeddedconfig"
)

const packageLogPrefix = "flashlight.config"

var (
	log = golog.LoggerFor(packageLogPrefix)

	// DefaultGlobalConfigPollInterval determines how frequently to fetch global.yaml
	DefaultGlobalConfigPollInterval = 1 * time.Hour
)

// Init determines the URLs at which to fetch global config and passes those to InitWithURLs, which
// initializes the config setup for fetching the global config. It returns a function that can be
// used to stop the reading of configs.
func Init(
	configDir string, flags map[string]interface{}, userConfig common.UserConfig,
	origGlobalDispatch func(interface{}, Source), onGlobalSaveError func(error),
	rt http.RoundTripper) (stop func()) {

	staging := isStaging(flags)
	globalConfigURL := checkOverrides(flags, getGlobalURL(staging), "global.yaml.gz")

	return InitWithURLs(
		configDir, flags, userConfig,
		origGlobalDispatch, onGlobalSaveError, globalConfigURL, rt)
}

type cfgWithSource struct {
	cfg interface{}
	src Source
}

// InitWithURLs initializes the config setup for fetching the global config from the given URL. It
// returns a function that can be used to stop the reading of configs.
func InitWithURLs(
	configDir string, flags map[string]interface{}, userConfig common.UserConfig,
	origGlobalDispatch func(interface{}, Source), onGlobalSaveError func(error),
	globalURL string, rt http.RoundTripper,
) (stop func()) {

	var mx sync.RWMutex
	globalConfigPollInterval := DefaultGlobalConfigPollInterval

	globalDispatchCh := make(chan cfgWithSource)
	go func() {
		for c := range globalDispatchCh {
			origGlobalDispatch(c.cfg, c.src)
		}
	}()

	globalDispatch := func(cfg interface{}, src Source) {
		globalConfig, ok := cfg.(*Global)
		if ok {
			mx.Lock()
			if globalConfig.GlobalConfigPollInterval > 0 {
				globalConfigPollInterval = globalConfig.GlobalConfigPollInterval
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
	url string, name string) string {
	if s, ok := flags["cloudconfig"].(string); ok {
		if len(s) > 0 {
			log.Debugf("Overridding config URL from the command line '%v'", s)
			return s + "/" + name
		}
	}
	return url
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
