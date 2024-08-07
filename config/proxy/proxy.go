// Package proxyconfig provides a simple way to manage client configuration. It reads and writes
// configuration to disk and provides a way to listen for changes.
package proxyconfig

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/getlantern/eventual/v2"
	"github.com/getlantern/golog"
	"github.com/getlantern/rot13"
	"google.golang.org/protobuf/proto"

	"github.com/getlantern/flashlight/v7/apipb"
)

const (
	defaultConfigSaveDir  = ""
	defaultConfigFilename = "proxies.conf"
)

// alias for better readability
type ProxyConfig = apipb.ConfigResponse

var (
	_config = &config{
		config: eventual.NewValue(),
	}

	log = golog.LoggerFor("proxyconfig")
)

type config struct {
	// config is the current client config as a *ProxyConfig.
	config eventual.Value

	// filePath is where we should save new configs and look for existing saved configs.
	filePath string
	// obfuscate specifies whether or not to obfuscate the config on disk.
	obfuscate bool

	// listeners is a list of functions to call when the config changes.
	listeners []func(old, new *ProxyConfig)
	mu        sync.Mutex
}

func Init(saveDir string, obfuscate bool) (*config, error) {
	if saveDir == "" {
		saveDir = defaultConfigSaveDir
		_config.filePath = filepath.Join(saveDir, defaultConfigFilename)
	}

	_config.obfuscate = obfuscate

	saved, err := readExistingConfig(_config.filePath, obfuscate)
	if err != nil {
		log.Errorf("Failed to read existing client config: %w", err)
	}

	if saved == nil {
		return _config, nil // no saved config
	}

	_config.config.Set(saved)
	notifyListeners(nil, saved)
	return _config, nil
}

// GetConfig implements services.ConfigHandler
func (c *config) GetConfig() *ProxyConfig {
	conf, _ := c.config.Get(eventual.DontWait)
	return conf.(*ProxyConfig)
}

// SetConfig implements services.ConfigHandler
func (c *config) SetConfig(new *ProxyConfig) {
	old := c.GetConfig()
	updated := new
	if old != nil {
		updated = proto.Clone(old).(*ProxyConfig)
		if new.Proxy != nil && len(new.Proxy.Proxies) > 0 {
			// We will always recieve the full list of proxy configs from the server if there are any
			// changes since we don't currently have a way to inform clients to delete an individual
			// proxy config. So we want to overwrite the existing proxy configs with the new ones.
			updated.Proxy = nil
		}

		proto.Merge(updated, new)
	}

	c.config.Set(updated)
	if err := saveConfig(c.filePath, updated, c.obfuscate); err != nil {
		log.Errorf("Failed to save client config: %w", err)
	}

	notifyListeners(old, updated)
}

// GetConfig returns the current client config. An error is returned if the config is not available
// within the given timeout.
func GetConfig(timeout time.Duration) (*ProxyConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conf, err := _config.config.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	if conf == nil {
		// This can only be due to a combination of unset config and expired context.
		return nil, fmt.Errorf("config not yet set: %w", ctx.Err())
	}

	return conf.(*ProxyConfig), nil
}

// readExistingConfig reads a config from a file at the specified path, filePath,
// deobfuscating it if obfuscate is true.
func readExistingConfig(filePath string, obfuscate bool) (*ProxyConfig, error) {
	infile, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // file does not exist
		}

		return nil, fmt.Errorf("unable to open config file %v for reading: %w", filePath, err)
	}
	defer infile.Close()

	var in io.Reader = infile
	if obfuscate {
		in = rot13.NewReader(infile)
	}

	bytes, err := io.ReadAll(in)
	if err != nil {
		return nil, fmt.Errorf("failed to read config from %v: %w", filePath, err)
	}

	if len(bytes) == 0 {
		return nil, nil // file is empty
	}

	conf := &ProxyConfig{}
	err = proto.Unmarshal(bytes, conf)
	return conf, err
}

// saveConfig writes conf to a file at the specified path, filePath, obfuscating it if
// obfuscate is true. If the file already exists, it will be overwritten.
func saveConfig(filePath string, conf *ProxyConfig, obfuscate bool) error {
	in, err := proto.Marshal(conf)
	if err != nil {
		return fmt.Errorf("unable to marshal config: %w", err)
	}

	outfile, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("unable to open file %v for writing: %w", filePath, err)
	}
	defer outfile.Close()

	var out io.Writer = outfile
	if obfuscate {
		out = rot13.NewWriter(outfile)
	}

	if _, err = out.Write(in); err != nil {
		return fmt.Errorf("unable to write config to file %v: %w", filePath, err)
	}

	return nil
}

// OnConfigChange registers a function to be called on config change. This allows callers to
// respond to each change without having to constantly poll for changes.
func OnConfigChange(fn func(old, new *ProxyConfig)) {
	_config.mu.Lock()
	if _config.listeners == nil {
		_config.listeners = make([]func(old, new *ProxyConfig), 0, 1)
	}

	_config.listeners = append(_config.listeners, fn)
	_config.mu.Unlock()
}

func notifyListeners(old, new *ProxyConfig) {
	_config.mu.Lock()
	new = proto.Clone(new).(*ProxyConfig)
	if old != nil {
		old = proto.Clone(old).(*ProxyConfig)
	}

	for _, fn := range _config.listeners {
		// don't block the config service
		go fn(old, new)
	}

	_config.mu.Unlock()
}
