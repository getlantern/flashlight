package config

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/rot13"
	"github.com/getlantern/tarfs"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/service"
)

var (
	log = golog.LoggerFor("flashlight.config")

	ServiceType service.Type = "flashlight.config"
)

type ConfigOpts struct {
	// SaveDir is the directory where we should save new configs and also look
	// for existing saved configs.
	SaveDir string
	// Obfuscate specifies whether or not to obfuscate the config on disk.
	Obfuscate bool
	// UserID specifies the user ID header to send to when fetching config.
	UserID string
	// Token specifies the token header to send to when fetching config.
	Token string
	// Sticky specifies whether or not fetch config. If true, don't fetch.
	Sticky bool
	// Global specifies the options to fetch global config.
	Global FetchOpts
	// Proxies specifies the options to fetch proxies config.
	Proxies FetchOpts
}

func (o *ConfigOpts) For() service.Type {
	return ServiceType
}

func (o *ConfigOpts) Complete() bool {
	return o.SaveDir != "" &&
		o.UserID != "" &&
		o.Token != "" &&
		o.Global.Complete() &&
		o.Proxies.Complete()
}

type FetchOpts struct {
	// YAMLName specifies the name of the config file both on disk and in the
	// embedded config that uses tarfs (the same in the interest of using
	// configuration by convention).
	YAMLName string
	// ChainedURL is the chained URL to use for fetching this config.
	ChainedURL string
	// FrontedURL is the fronted URL to use for fetching this config.
	FrontedURL string
	// EmbeddedData is the data for embedded configs, using tarfs.
	EmbeddedData []byte
	// FetchInteval is the time between config fetches.
	FetchInteval time.Duration
}

func (o *FetchOpts) Complete() bool {
	return o.YAMLName != "" &&
		o.ChainedURL != "" &&
		o.FrontedURL != "" &&
		o.EmbeddedData != nil &&
		o.FetchInteval > 0
}

// config gets proxy data saved locally, embedded in the binary, or fetched
// over the network.
type config struct {
	filePath    string
	obfuscate   bool
	saveChan    chan interface{}
	unmarshaler func([]byte) (interface{}, error)
}

// chainedFrontedURLs contains a chained and a fronted URL for fetching a config.
type chainedFrontedURLs struct {
	chained string
	fronted string
}

// options specifies the options to use for piping config data back to the
// dispatch processor function.
type options struct {

	// saveDir is the directory where we should save new configs and also look
	// for existing saved configs.
	saveDir string

	// obfuscate specifies whether or not to obfuscate the config on disk.
	obfuscate bool

	// name specifies the name of the config file both on disk and in the
	// embedded config that uses tarfs (the same in the interest of using
	// configuration by convention).
	name string

	// urls are the chaines and fronted URLs to use for fetching this config.
	urls *chainedFrontedURLs

	// userConfig contains data for communicating the user details to upstream
	// servers in HTTP headers, such as the pro token.
	userConfig UserConfig

	//  unmarshaler unmarshals application specific data structure.
	unmarshaler func([]byte) (interface{}, error)

	// dispatch is essentially a callback function for processing retrieved
	// yaml configs.
	dispatch func(cfg interface{})

	// embeddedData is the data for embedded configs, using tarfs.
	embeddedData []byte

	// sleep the time to sleep between config fetches.
	sleep time.Duration

	// sticky specifies whether or not to only use the local config and not
	// update it with remote data.
	sticky bool
}

// pipeConfig creates a new config pipeline for reading a specified type of
// config onto a channel for processing by a dispatch function. This will read
// configs in the following order:
//
// 1. Configs saved on disk, if any
// 2. Configs embedded in the binary according to the specified name, if any.
// 3. Configs fetched remotely, and those will be piped back over and over
//   again as the remote configs change (but only if they change).
func pipeConfig(opts *options) {

	configChan := make(chan interface{})

	go func() {
		for {
			cfg := <-configChan
			opts.dispatch(cfg)
		}
	}()
	configPath, err := client.InConfigDir(opts.saveDir, opts.name)
	if err != nil {
		log.Errorf("Could not get config path? %v", err)
	}

	log.Tracef("Obfuscating %v", opts.obfuscate)
	conf := newConfig(configPath, opts)

	if saved, proxyErr := conf.saved(); proxyErr != nil {
		log.Debugf("Could not load stored config %v", proxyErr)
		if embedded, errr := conf.embedded(opts.embeddedData, opts.name); errr != nil {
			log.Errorf("Could not load embedded config %v", errr)
		} else {
			log.Debugf("Sending embedded config for %v", opts.name)
			configChan <- embedded
		}
	} else {
		log.Debugf("Sending saved config for %v", opts.name)
		configChan <- saved
	}

	// Now continually poll for new configs and pipe them back to the dispatch
	// function.
	if !opts.sticky {
		go conf.poll(opts.userConfig, configChan, opts.urls, opts.sleep)
	} else {
		log.Debugf("Using sticky config")
	}
}

// newConfig create a new ProxyConfig instance that saves and looks for
// saved data at the specified path.
func newConfig(filePath string,
	opts *options,
) *config {
	cfg := &config{
		filePath:    filePath,
		obfuscate:   opts.obfuscate,
		saveChan:    make(chan interface{}),
		unmarshaler: opts.unmarshaler,
	}
	if cfg.unmarshaler == nil {
		cfg.unmarshaler = func([]byte) (interface{}, error) {
			return nil, errors.New("No unmarshaler")
		}
	}

	// Start separate go routine that saves newly fetched proxies to disk.
	go cfg.save()
	return cfg
}

// saved returns a yaml config from disk.
func (conf *config) saved() (interface{}, error) {
	infile, err := os.Open(conf.filePath)
	if err != nil {
		err = fmt.Errorf("Unable to open config file %v for reading: %v", conf.filePath, err)
		log.Error(err.Error())
		return nil, err
	}
	defer infile.Close()

	var in io.Reader = infile
	if conf.obfuscate {
		in = rot13.NewReader(infile)
	}

	bytes, err := ioutil.ReadAll(in)
	if err != nil {
		err = fmt.Errorf("Error reading config from %v: %v", conf.filePath, err)
		log.Error(err.Error())
		return nil, err
	}
	if len(bytes) == 0 {
		return nil, fmt.Errorf("Config exists but is empty at %v", conf.filePath)
	}

	log.Debugf("Returning saved config at %v", conf.filePath)
	return conf.unmarshaler(bytes)
}

// embedded retrieves a yaml config embedded in the binary.
func (conf *config) embedded(data []byte, fileName string) (interface{}, error) {
	fs, err := tarfs.New(data, "")
	if err != nil {
		log.Errorf("Could not read resources? %v", err)
		return nil, err
	}

	// Get the yaml file from either the local file system or from an
	// embedded resource, but ignore local file system files if they're
	// empty.
	bytes, err := fs.GetIgnoreLocalEmpty(fileName)
	if err != nil {
		log.Errorf("Could not read embedded proxies %v", err)
		return nil, err
	}

	return conf.unmarshaler(bytes)
}

// Poll polls for new configs from a remote server and saves them to disk for
// future runs.
func (conf *config) poll(uc UserConfig,
	configChan chan interface{}, urls *chainedFrontedURLs, sleep time.Duration) {
	fetcher := newFetcher(uc, proxied.ParallelPreferChained(), urls)

	for {
		if bytes, err := fetcher.fetch(); err != nil {
			log.Errorf("Error fetching config: %v", err)
		} else if bytes == nil {
			// This is what fetcher returns for not-modified.
			log.Debug("Ignoring not modified response")
		} else if cfg, err := conf.unmarshaler(bytes); err != nil {
			log.Errorf("Error fetching config: %v", err)
		} else {
			log.Debugf("Fetched config! %v", cfg)

			// Push these to channels to avoid race conditions that might occur if
			// we did these on go routines, for example.
			conf.saveChan <- cfg
			log.Debugf("Sent to save chan")
			configChan <- cfg
		}
		time.Sleep(sleep)
	}
}

func (conf *config) save() {
	for {
		in := <-conf.saveChan
		if err := conf.saveOne(in); err != nil {
			log.Errorf("Could not save %v, %v", in, err)
		}
	}
}

func (conf *config) saveOne(in interface{}) error {
	op := ops.Begin("save_config")
	defer op.End()
	return op.FailIf(conf.doSaveOne(in))
}

func (conf *config) doSaveOne(in interface{}) error {
	bytes, err := yaml.Marshal(in)
	if err != nil {
		return fmt.Errorf("Unable to marshal config yaml: %v", err)
	}

	outfile, err := os.OpenFile(conf.filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("Unable to open file %v for writing: %v", conf.filePath, err)
	}
	defer outfile.Close()

	var out io.Writer = outfile
	if conf.obfuscate {
		out = rot13.NewWriter(outfile)
	}
	_, err = out.Write(bytes)
	if err != nil {
		return fmt.Errorf("Unable to write yaml to file %v: %v", conf.filePath, err)
	}
	log.Debugf("Wrote file at %v", conf.filePath)
	return nil
}
