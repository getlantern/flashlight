// Description: This file contains the implementation of the new consolidated proxy/geolocation config.

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
	"github.com/getlantern/lantern-cloud/cmd/api/apipb"
	"github.com/getlantern/rot13"
	"google.golang.org/protobuf/proto"
)

const (
	defaultConfigSaveDir  = ""
	defaultConfigFilename = "proxies.conf"
)

// aliases for better readability
type (
	ProxyConfig        = apipb.ConfigResponse
	ProxyConnectConfig = apipb.ProxyConnectConfig
)

var (
	_config = &config{
		config: eventual.NewValue(),
	}

	log = golog.LoggerFor("proxyconfig")
)

type config struct {
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
	c.config.Set(new)
	if err := saveConfig(c.filePath, new, c.obfuscate); err != nil {
		log.Errorf("Failed to save client config: %w", err)
	}

	notifyListeners(old, new)
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

	return conf.(*ProxyConfig), nil
}

// GetProToken returns the pro token from the current client config. An error is returned if the
// config is not available within the given timeout.
func GetProToken(timeout time.Duration) (string, error) {
	config, err := GetConfig(timeout)
	if err != nil || config == nil {
		return "", err
	}

	return config.GetProToken(), nil
}

// GetProxyConfigs returns the list of proxy configs from the current client config. An error is
// returned if the config is not available within the given timeout.
func GetProxyConfigs(timeout time.Duration) ([]*ProxyConnectConfig, error) {
	config, err := GetConfig(timeout)
	if err != nil || config == nil {
		return nil, err
	}

	return config.GetProxy().GetProxies(), nil
}

// readExistingConfig reads a config from a file at the specified path, filePath,
// deobfuscating it if obfuscate is true.
func readExistingConfig(filePath string, obfuscate bool) (*ProxyConfig, error) {
	infile, err := os.Open(filePath)
	if err != nil {
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
	listeners := _config.listeners
	_config.mu.Unlock()
	// TODO: should we clone the configs before passing them to the listeners?
	for _, fn := range listeners {
		// don't block the config service
		go fn(old, new)
	}
}
