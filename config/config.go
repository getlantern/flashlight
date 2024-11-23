package config

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/getlantern/golog"
	"github.com/getlantern/rot13"

	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/embeddedconfig"
	"github.com/getlantern/flashlight/v7/ops"
)

// Source specifies where the config is from when dispatching.
type Source string

const (
	Saved    Source = "saved"
	Embedded Source = "embedded"
	Fetched  Source = "fetched"
)

// Config is an interface for getting proxy data saved locally, embedded
// in the binary, or fetched over the network.
type Config interface {

	// Saved returns a yaml config from disk.
	saved() (interface{}, error)

	// Embedded retrieves a yaml config embedded in the binary.
	embedded([]byte) (interface{}, error)

	// Polls for new configs from a remote server and saves them to disk for future runs.
	configFetcher(stopCh chan bool, dispatch func(interface{}), fetcher Fetcher, sleep func() time.Duration)
}

type config struct {
	filePath    string
	obfuscate   bool
	saveChan    chan []byte
	unmarshaler func([]byte) (interface{}, error)
}

// options specifies the options to use for piping config data back to the
// dispatch processor function.
type options struct {

	// saveDir is the directory where we should save new configs and also look
	// for existing saved configs.
	saveDir string

	// onSaveError is called when an error occurs saving the config. May be nil.
	onSaveError func(error)

	// obfuscate specifies whether or not to obfuscate the config on disk.
	obfuscate bool

	// name specifies the name of the config file both on disk and in the
	// embedded config that uses tarfs (the same in the interest of using
	// configuration by convention).
	name string

	// URL to use for fetching this config.
	originURL string

	// userConfig contains data for communicating the user details to upstream
	// servers in HTTP headers, such as the pro token and other options.
	userConfig common.UserConfig

	// marshaler marshals application specific config to bytes, defaults to
	// yaml.Marshal
	marshaler func(interface{}) ([]byte, error)

	//  unmarshaler unmarshals application specific data structure.
	unmarshaler func([]byte) (interface{}, error)

	// dispatch is essentially a callback function for processing retrieved
	// yaml configs.
	dispatch func(cfg interface{}, src Source)

	// sleep the time to sleep between config fetches.
	sleep func() time.Duration

	// sticky specifies whether or not to only use the local config and not
	// update it with remote data.
	sticky bool

	// rt provides the RoundTripper the fetcher should use, which allows us to
	// dictate whether the fetcher will use dual fetching (from fronted and
	// chained URLs) or not.
	rt http.RoundTripper

	// opName is the operation name to use for ops.Begin when fetching configs.
	opName string

	// ignoreSaved specifies whether or not to ignore saved config and only use
	// configs fetched from the network (possibly because we previously snagged
	// the saved config, for example).
	ignoreSaved bool
}

func GlobalConfig(configDir string, flags map[string]interface{}) (*Global, error) {
	configPath := filepath.Join(configDir, "global.yaml")
	embedded := NewGlobal()
	if err := yaml.Unmarshal(embeddedconfig.Global, embedded); err != nil {
		return nil, err
	}
	if err := embedded.Validate(); err != nil {
		return nil, err
	}

	saved, err := savedGlobal(configPath, obfuscate(flags))
	if err != nil {
		return embedded, nil
	}

	// The embedded config can be newer, for example, if the user has installed a new version of the app.
	if embeddedIsNewer(configPath) {
		return embedded, nil
	} else {
		return saved, nil
	}
}

func savedGlobal(filePath string, obfuscate bool) (*Global, error) {
	bytes, err := saved(filePath, obfuscate)
	if err != nil {
		return nil, err
	}

	log.Debugf("Returning saved global config at %v", filePath)
	saved := NewGlobal()
	if err := yaml.Unmarshal(bytes, saved); err != nil {
		return nil, err
	}
	if err := saved.Validate(); err != nil {
		return nil, err
	}
	return saved, nil
}

// pipeConfig creates a new config pipeline for reading a specified type of
// config onto a channel for processing by a dispatch function. This will read
// configs in the following order:
//
//  1. Configs saved on disk, if any
//  2. Configs embedded in the binary according to the specified name, if any.
//  3. Configs fetched remotely, and those will be piped back over and over
//     again as the remote configs change (but only if they change).
//
// pipeConfig returns a function that can be used to stop polling
func pipeConfig(opts *options) (stop func()) {
	stopCh := make(chan bool)

	// lastCfg is accessed by both the current goroutine when dispatching
	// saved or embedded configs, and in a separate goroutine for polling
	// for remote configs. There should never be mutual access by these
	// goroutines, however, since the polling routine is started after the prior
	// calls to dispatch return.
	var (
		mu      sync.Mutex
		lastCfg interface{}
	)
	dispatch := func(cfg interface{}, src Source) {
		// It's not clear exactly how atomic this needs to be. I think we also need to synchronize
		// on the dispatch and lastCfg update, further updates will have to wait until the current
		// dispatch is completed.
		mu.Lock()
		defer mu.Unlock()
		a := lastCfg
		b := yamlRoundTrip(cfg)
		if reflect.DeepEqual(a, b) {
			log.Debug("Config unchanged, ignoring")
		} else {
			log.Debugf("Dispatching updated config from %v", src)
			opts.dispatch(cfg, src)
			lastCfg = b
		}
	}

	configPath := filepath.Join(opts.saveDir, opts.name)
	conf := newConfig(configPath, opts)

	// We ignore the saved global config here because we need it so early in the
	// initialization sequence that we've already grabbed it at this point.
	if !opts.ignoreSaved {
		if saved, err := saved(configPath, conf.obfuscate); err != nil {
			log.Debugf("Could not load stored config %v", err)
		} else {
			log.Debugf("Sending saved config for %v", opts.name)
			dispatch(saved, Saved)
		}
	}

	// Now continually poll for new configs and pipe them back to the dispatch function.
	if !opts.sticky {
		fetcher := newHttpFetcher(opts.userConfig, opts.rt, opts.originURL)
		go conf.configFetcher(opts.opName, stopCh,
			func(cfg interface{}) {
				dispatch(cfg, Fetched)
			}, fetcher, opts.sleep,
			golog.LoggerFor(fmt.Sprintf("%v.%v.fetcher.http", packageLogPrefix, opts.name)))
	} else {
		log.Debugf("Using sticky config")
	}

	return func() {
		close(stopCh)
	}
}

// Checks to see if our embedded config is newer than our saved config. If it is, use that. This could happen,
// for example, if the user has successfully auto-updated or installed a new version by any means, but
// where there's some blocking event or bug preventing new configs from being fetched.
func embeddedIsNewer(savedPath string) bool {
	saved, err := os.Stat(savedPath)
	if os.IsNotExist(err) {
		return true
	}

	exePath, err := os.Executable()
	if err != nil {
		return false
	}

	if exe, err := os.Stat(exePath); err != nil {
		return false
	} else {
		return saved.ModTime().Before(exe.ModTime())
	}
}

func yamlRoundTrip(o interface{}) interface{} {
	if o == nil {
		return nil
	}
	var or interface{}
	t := reflect.TypeOf(o)
	if t.Kind() == reflect.Ptr {
		or = reflect.New(t.Elem()).Interface()
	} else {
		or = reflect.New(t).Interface()
	}
	b, err := yaml.Marshal(o)
	if err != nil {
		log.Errorf("Unable to yaml round trip (marshal): %v %v", o, err)
		return o
	}
	err = yaml.Unmarshal(b, or)
	if err != nil {
		log.Errorf("Unable to yaml round trip (unmarshal): %v %v", o, err)
		return o
	}
	return or
}

// newConfig create a new ProxyConfig instance that saves and looks for
// saved data at the specified path.
func newConfig(filePath string, opts *options) *config {
	cfg := &config{
		filePath:    filePath,
		obfuscate:   opts.obfuscate,
		saveChan:    make(chan []byte),
		unmarshaler: opts.unmarshaler,
	}
	if cfg.unmarshaler == nil {
		cfg.unmarshaler = func([]byte) (interface{}, error) {
			return nil, errors.New("no unmarshaler")
		}
	}

	// Start separate go routine that saves newly fetched proxies to disk.
	go cfg.save(opts.onSaveError)
	return cfg
}

func saved(filePath string, obfuscate bool) ([]byte, error) {
	infile, err := os.Open(filePath)
	if err != nil {
		err = fmt.Errorf("unable to open config file %v for reading: %w", filePath, err)
		log.Error(err.Error())
		return nil, err
	}
	defer infile.Close()

	var in io.Reader = infile
	if obfuscate {
		in = rot13.NewReader(infile)
	}

	bytes, err := io.ReadAll(in)
	if err != nil {
		err = fmt.Errorf("error reading config from %v: %w", filePath, err)
		log.Error(err.Error())
		return nil, err
	}
	if len(bytes) == 0 {
		return nil, fmt.Errorf("config exists but is empty at %v", filePath)
	}

	log.Debugf("Returning saved config at %v", filePath)
	return bytes, nil
	//return conf.unmarshaler(bytes)
}

func (conf *config) embedded(data []byte) (interface{}, error) {
	return conf.unmarshaler(data)
}

func (conf *config) configFetcher(opName string, stopCh chan bool, dispatch func(interface{}), fetcher Fetcher, defaultSleep func() time.Duration, log golog.Logger) {
	for {
		sleepDuration := conf.fetchConfig(opName, stopCh, dispatch, fetcher, log)
		if sleepDuration == noSleep {
			sleepDuration = defaultSleep()
		}
		select {
		case <-stopCh:
			log.Debug("Stopping polling")
			return
		case <-time.After(sleepDuration):
			continue
		}
	}
}

func (conf *config) fetchConfig(opName string, stopCh chan bool, dispatch func(interface{}), fetcher Fetcher, log golog.Logger) time.Duration {
	if bytes, sleepTime, err := fetcher.fetch(opName); err != nil {
		log.Errorf("Error fetching config: %v", err)
		return sleepTime
	} else if bytes == nil {
		// This is what fetcher returns for not-modified.
		log.Debug("Ignoring not modified response")
		return sleepTime
	} else if cfg, err := conf.unmarshaler(bytes); err != nil {
		log.Errorf("Error fetching config: %v", err)
		return sleepTime
	} else {
		log.Debugf("Fetched config!")

		// At this point we know the raw bytes were successfully decoded as valid yaml, so just save them directly.
		conf.saveChan <- bytes
		log.Debug("Sent to save chan")
		dispatch(cfg)
		return sleepTime
	}
}

func (conf *config) save(onError func(error)) {
	for {
		in := <-conf.saveChan
		if err := conf.saveOne(in); err != nil && onError != nil {
			// Handle the error in a goroutine to avoid blocking the save loop.
			go onError(err)
		}
	}
}

func (conf *config) saveOne(in []byte) error {
	op := ops.Begin("save_config")
	defer op.End()
	return op.FailIf(conf.doSaveOne(in))
}

func (conf *config) doSaveOne(in []byte) error {
	outfile, err := os.OpenFile(conf.filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("unable to open file %v for writing: %w", conf.filePath, err)
	}
	defer outfile.Close()

	var out io.Writer = outfile
	if conf.obfuscate {
		out = rot13.NewWriter(outfile)
	}
	_, err = out.Write(in)
	if err != nil {
		return fmt.Errorf("unable to write yaml to file %v: %w", conf.filePath, err)
	}
	log.Debugf("Wrote file at %v", conf.filePath)
	return nil
}
