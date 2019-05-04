package config

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"time"

	"github.com/getlantern/rot13"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
)

// Config is an interface for getting proxy data saved locally, embedded
// in the binary, or fetched over the network.
type Config interface {

	// Saved returns a yaml config from disk.
	saved() (interface{}, error)

	// Embedded retrieves a yaml config embedded in the binary.
	embedded([]byte) (interface{}, error)

	// Poll polls for new configs from a remote server and saves them to disk for
	// future runs.
	poll(stopCh chan bool, dispatch func(interface{}), fetcher Fetcher, sleep func() time.Duration)
}

type config struct {
	filePath    string
	obfuscate   bool
	saveChan    chan interface{}
	unmarshaler func([]byte) (interface{}, error)
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
	dispatch func(cfg interface{})

	// embeddedData is the data for embedded configs, using tarfs.
	embeddedData []byte

	// sleep the time to sleep between config fetches.
	sleep func() time.Duration

	// sticky specifies whether or not to only use the local config and not
	// update it with remote data.
	sticky bool

	// rt provides the RoundTripper the fetcher should use, which allows us to
	// dictate whether the fetcher will use dual fetching (from fronted and
	// chained URLs) or not.
	rt http.RoundTripper
}

// pipeConfig creates a new config pipeline for reading a specified type of
// config onto a channel for processing by a dispatch function. This will read
// configs in the following order:
//
// 1. Configs saved on disk, if any
// 2. Configs embedded in the binary according to the specified name, if any.
// 3. Configs fetched remotely, and those will be piped back over and over
//   again as the remote configs change (but only if they change).
//
// pipeConfig returns a function that can be used to stop polling
func pipeConfig(opts *options) (stop func()) {
	stopCh := make(chan bool)

	// lastCfg is accessed by both the current goroutine when dispatching
	// saved or embedded configs, and in a separate goroutine for polling
	// for remote configs.  There should never be mutual access by these
	// goroutines, however, since the polling routine is started after the prior
	// calls to dispatch return.
	var lastCfg interface{}
	dispatch := func(cfg interface{}) {
		a := lastCfg
		b := yamlRoundTrip(cfg)
		if reflect.DeepEqual(a, b) {
			log.Debug("Config unchanged, ignoring")
		} else {
			log.Debug("Dispatching updated config")
			opts.dispatch(cfg)
			lastCfg = b
		}
	}

	configPath, err := common.InConfigDir(opts.saveDir, opts.name)
	if err != nil {
		log.Errorf("Could not get config path? %v", err)
	}

	log.Tracef("Obfuscating %v", opts.obfuscate)
	conf := newConfig(configPath, opts)

	if saved, proxyErr := conf.saved(); proxyErr != nil {
		log.Debugf("Could not load stored config %v", proxyErr)
		if embedded, errr := conf.embedded(opts.embeddedData); errr != nil {
			log.Errorf("Could not load embedded config %v", errr)
		} else {
			log.Debugf("Sending embedded config for %v", opts.name)
			dispatch(embedded)
		}
	} else {
		log.Debugf("Sending saved config for %v", opts.name)
		dispatch(saved)
	}

	// Now continually poll for new configs and pipe them back to the dispatch
	// function.
	if !opts.sticky {
		fetcher := newFetcher(opts.userConfig, opts.rt, opts.originURL)
		go conf.poll(stopCh, dispatch, fetcher, opts.sleep)
	} else {
		log.Debugf("Using sticky config")
	}

	return func() {
		close(stopCh)
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
func newConfig(filePath string,
	opts *options,
) Config {
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

func (conf *config) embedded(data []byte) (interface{}, error) {
	return conf.unmarshaler(data)
}

func (conf *config) poll(stopCh chan bool, dispatch func(interface{}), fetcher Fetcher, sleep func() time.Duration) {
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
			dispatch(cfg)
		}
		select {
		case <-stopCh:
			log.Debug("Stopping polling")
			return
		case <-time.After(sleep()):
			continue
		}
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
