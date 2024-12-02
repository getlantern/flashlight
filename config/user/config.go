// Package userconfig provides a simple way to manage client configuration. It reads and writes
// configuration to disk and provides a way to listen for changes.
package userconfig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/getlantern/eventual/v2"
	"github.com/getlantern/golog"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/getlantern/flashlight/v7/apipb"
)

const (
	DefaultConfigSaveDir  = ""
	DefaultConfigFilename = "user.conf"
)

// alias for better readability.
// Using "UserConfig" since "Config" and "ClientConfig" is already taken. This may change in the
// future if they become available.
type UserConfig = apipb.ConfigResponse

var (
	_config = &Config{
		config: eventual.NewValue(),
	}
	initCalled = atomic.Bool{}
	log        = golog.LoggerFor("userconfig")
)

type Config struct {
	// config is the current client config as a *UserConfig.
	config eventual.Value

	// filePath is where we should save new configs and look for existing saved configs.
	filePath string
	// readable specifies whether the config file should be saved in a human-readable format.
	readable bool

	// listeners is a list of functions to call when the config changes.
	listeners []func(old, new *UserConfig)
	mu        sync.Mutex
}

func Init(saveDir string, readable bool) (*Config, error) {
	if !initCalled.CompareAndSwap(false, true) {
		return _config, nil
	}

	if saveDir == "" {
		saveDir = DefaultConfigSaveDir
	}

	return initialize(saveDir, DefaultConfigFilename, readable)
}

func initialize(saveDir, filename string, readable bool) (*Config, error) {
	_config.mu.Lock()
	_config.filePath = filepath.Join(saveDir, filename)
	_config.readable = readable
	_config.mu.Unlock()

	saved, err := readExistingConfig(_config.filePath, readable)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	if saved == nil {
		log.Debug("No existing userconfig found")
		return _config, nil // no saved config
	}

	log.Debug("Loaded saved config")
	_config.config.Set(saved)
	notifyListeners(nil, saved)
	return _config, nil
}

// GetConfig implements services.ConfigHandler
func (c *Config) GetConfig() *UserConfig {
	v, _ := c.config.Get(eventual.DontWait)
	conf, _ := v.(*UserConfig)
	return conf
}

// SetConfig implements services.ConfigHandler
func (c *Config) SetConfig(new *UserConfig) {
	log.Debug("Setting user config")
	old := c.GetConfig()
	updated := new
	if old != nil {
		updated = proto.Clone(old).(*UserConfig)
		if new.GetProxy() != nil && len(new.GetProxy().GetProxies()) > 0 {
			// We will always recieve the full list of proxy configs from the server if there are any
			// changes since we don't currently have a way to inform clients to delete an individual
			// proxy config. So we want to overwrite the existing proxy configs with the new ones.
			updated.Proxy = nil
		}

		proto.Merge(updated, new)
	}

	log.Tracef("Config changed:\nold:\n%+v\nnew:\n%+v\nmerged:\n%v", old, new, updated)

	c.config.Set(updated)
	if err := saveConfig(c.filePath, updated, c.readable); err != nil {
		log.Errorf("Failed to save client config: %v", err)
	}

	notifyListeners(old, updated)
}

// GetConfig returns the current client config. An error is returned if the config is not available
// within the given timeout.
func GetConfig(ctx context.Context) (*UserConfig, error) {
	conf, err := _config.config.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	if conf == nil {
		// This can only be due to a combination of unset config and expired context.
		return nil, fmt.Errorf("config not yet set: %w", ctx.Err())
	}

	return conf.(*UserConfig), nil
}

// readExistingConfig reads a config from a file at filePath. readable specifies whether the file
// is in JSON format. A nil error is returned even if the file does not exist or is empty as these
// are not considered errors.
func readExistingConfig(filePath string, readable bool) (*UserConfig, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // file does not exist
		}

		return nil, fmt.Errorf("unable to open config file %v for reading: %w", filePath, err)
	}

	if len(bytes) == 0 { // file is empty
		// we treat an empty file as if it doesn't exist
		return nil, nil
	}

	conf := &UserConfig{}
	if readable {
		err = protojson.Unmarshal(bytes, conf)
	} else {
		err = proto.Unmarshal(bytes, conf)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal config: %w", err)
	}

	return conf, nil
}

// saveConfig writes conf to a file at filePath. If readable is true, the file will be written in
// JSON format. Otherwise, it will be written in protobuf format. If the file already exists, it
// will be overwritten.
func saveConfig(filePath string, conf *UserConfig, readable bool) error {
	var (
		buf []byte
		err error
	)
	if readable {
		buf, err = protojson.Marshal(conf)
	} else {
		buf, err = proto.Marshal(conf)
	}

	if err != nil {
		return fmt.Errorf("unable to marshal config: %w", err)
	}

	return os.WriteFile(filePath, buf, 0644)
}

// OnConfigChange registers a function to be called on config change. This allows callers to
// respond to each change without having to constantly poll for changes.
func OnConfigChange(fn func(old, new *UserConfig)) {
	_config.mu.Lock()
	if _config.listeners == nil {
		_config.listeners = make([]func(old, new *UserConfig), 0, 1)
	}

	_config.listeners = append(_config.listeners, fn)
	_config.mu.Unlock()
}

func notifyListeners(old, new *UserConfig) {
	log.Trace("Notifying listeners")

	_config.mu.Lock()
	new = proto.Clone(new).(*UserConfig)
	if old != nil {
		old = proto.Clone(old).(*UserConfig)
	}

	for _, fn := range _config.listeners {
		// don't block the config service
		go fn(old, new)
	}

	_config.mu.Unlock()
}
