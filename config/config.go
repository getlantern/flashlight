package config

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/rot13"
	"github.com/getlantern/tarfs"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/chained"
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
	// OverrideGlobal, if supplied, alters the global config before publishing
	// it.
	OverrideGlobal func(*Global)
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
		o.Global.Complete() &&
		o.Proxies.Complete()
}

type FetchOpts struct {
	// FileName specifies the name of the config file on disk
	FileName string
	// EmbeddedName specifies the name of the embedded config that uses tarfs.
	EmbeddedName string
	// EmbeddedData is the data for embedded configs, using tarfs.
	EmbeddedData []byte
	// ChainedURL is the chained URL to use for fetching this config.
	ChainedURL string
	// FrontedURL is the fronted URL to use for fetching this config.
	FrontedURL string
	// FetchInteval is the time between config fetches.
	FetchInteval time.Duration
	// If true, hit all the way to the origin server which handles Lantern
	// special Etag.
	useLanternEtag bool
	unmarshaler    func(bytes []byte) (service.Message, error)
	// shared channel between fetcher and saver. Closed by the fetcher when
	// stopping.
	saveChan chan service.Message
	// fullPath of the file on disk.
	fullPath string
}

func (o *FetchOpts) Complete() bool {
	return o.FileName != "" &&
		o.EmbeddedName != "" &&
		o.EmbeddedData != nil &&
		o.ChainedURL != "" &&
		o.FrontedURL != "" &&
		o.FetchInteval > 0
}

type Proxies map[string]*chained.ChainedServerInfo

func (m Proxies) ValidMessageFrom(t service.Type) bool {
	return t == ServiceType
}

// config gets proxy data saved locally, embedded in the binary, or fetched
// over the network.
type config struct {
	publisher service.Publisher
	opts      *ConfigOpts
	chStop    chan bool
}

func New() service.Impl {
	return &config{}
}

func (c *config) GetType() service.Type {
	return ServiceType
}

func (c *config) Reconfigure(p service.Publisher, opts service.ConfigOpts) {
	c.publisher = p
	c.opts = opts.(*ConfigOpts)
}

func (c *config) Start() {
	c.chStop = make(chan bool)
	c.opts.Global.unmarshaler = c.unmarshalGlobal
	c.opts.Global.saveChan = make(chan service.Message)
	c.opts.Global.fullPath = path.Join(c.opts.SaveDir, c.opts.Global.FileName)
	c.loadInitial(&c.opts.Global)

	c.opts.Proxies.unmarshaler = c.unmarshalProxies
	c.opts.Proxies.saveChan = make(chan service.Message)
	c.opts.Proxies.fullPath = path.Join(c.opts.SaveDir, c.opts.Proxies.FileName)
	c.opts.Proxies.useLanternEtag = true
	c.loadInitial(&c.opts.Proxies)

	if !c.opts.Sticky {
		err := os.MkdirAll(c.opts.SaveDir, 0750)
		if err != nil {
			log.Errorf("Couldn't create dir %s: %v", c.opts.SaveDir, err)
			// continue and let the saver keep reporting error
		}
		go (&saver{c.opts.Global.saveChan, c.opts.Global.fullPath, c.opts.Obfuscate}).run()
		go c.poll(&c.opts.Global)

		go (&saver{c.opts.Proxies.saveChan, c.opts.Proxies.fullPath, c.opts.Obfuscate}).run()
		go c.poll(&c.opts.Proxies)
	}
}

func (c *config) Stop() {
	close(c.chStop)
}

func (c *config) loadInitial(opts *FetchOpts) {
	msg, err := c.saved(opts)
	if err == nil {
		log.Debugf("Sending saved config for %v", opts.fullPath)
		c.publisher.Publish(msg)
		return
	}
	log.Debugf("Could not load stored config %v", err)
	msg, err = c.embedded(opts)
	if err != nil {
		panic(fmt.Sprintf("Could not load embedded config %v", err))
	}
	log.Debugf("Sending saved config for %v", opts.EmbeddedName)
	c.publisher.Publish(msg)
}

// saved returns a yaml config from disk.
func (c *config) saved(opts *FetchOpts) (service.Message, error) {
	infile, err := os.Open(opts.fullPath)
	if err != nil {
		err = fmt.Errorf("Unable to open config file %v for reading: %v", opts.fullPath, err)
		log.Error(err.Error())
		return nil, err
	}
	defer infile.Close()

	var in io.Reader = infile
	if c.opts.Obfuscate {
		in = rot13.NewReader(infile)
	}

	bytes, err := ioutil.ReadAll(in)
	if err != nil {
		err = fmt.Errorf("Error reading config from %v: %v", opts.fullPath, err)
		log.Error(err.Error())
		return nil, err
	}
	if len(bytes) == 0 {
		return nil, fmt.Errorf("Config exists but is empty at %v", opts.fullPath)
	}

	log.Debugf("Returning saved config at %v", opts.fullPath)
	return opts.unmarshaler(bytes)
}

// embedded retrieves a yaml config embedded in the binary.
func (c *config) embedded(opts *FetchOpts) (service.Message, error) {
	fs, err := tarfs.New(opts.EmbeddedData, "")
	if err != nil {
		log.Errorf("Could not read resources? %v", err)
		return nil, err
	}

	// Get the yaml file from either the local file system or from an
	// embedded resource, but ignore local file system files if they're
	// empty.
	bytes, err := fs.GetIgnoreLocalEmpty(opts.EmbeddedName)
	if err != nil {
		log.Errorf("Could not read embedded proxies %v", err)
		return nil, err
	}

	return opts.unmarshaler(bytes)
}

// Poll polls for new configs from a remote server and saves them to disk for
// future runs.
func (c *config) poll(opts *FetchOpts) {
	fetcher := newFetcher(proxied.ParallelPreferChained(),
		opts.useLanternEtag,
		c.opts.UserID,
		c.opts.Token,
		opts.ChainedURL,
		opts.FrontedURL)

	for {
		if bytes, err := fetcher.fetch(); err != nil {
			log.Errorf("Error fetching config: %v", err)
		} else if bytes == nil {
			// This is what fetcher returns for not-modified.
			log.Debug("Ignoring not modified response")
		} else if cfg, err := opts.unmarshaler(bytes); err != nil {
			log.Errorf("Error unmarshalling config: %v", err)
		} else {
			log.Debugf("Fetched config! %v", cfg)

			// Push these to channels to avoid race conditions that might occur if
			// we did these on goroutines, for example.
			opts.saveChan <- cfg
			log.Debugf("Sent to save chan")
			c.publisher.Publish(cfg)
		}
		select {
		case <-c.chStop:
			return
		case <-time.After(opts.FetchInteval):
		}
	}
}

func (c *config) unmarshalGlobal(bytes []byte) (service.Message, error) {
	gl := newGlobal()
	if err := yaml.Unmarshal(bytes, gl); err != nil {
		return nil, err
	}
	if err := gl.validate(); err != nil {
		return nil, err
	}
	if c.opts.OverrideGlobal != nil {
		c.opts.OverrideGlobal(gl)
	}
	return gl, nil
}

func (c *config) unmarshalProxies(bytes []byte) (service.Message, error) {
	servers := make(map[string]*chained.ChainedServerInfo)
	if err := yaml.Unmarshal(bytes, servers); err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, errors.New("No chained server")
	}
	return Proxies(servers), nil
}

type saver struct {
	ch        chan service.Message
	fullPath  string
	obfuscate bool
}

func (s *saver) run() {
	for in := range s.ch {
		if err := s.saveOne(in); err != nil {
			log.Errorf("Could not save %v: %v", in, err)
		}
	}
}

func (s *saver) saveOne(in service.Message) error {
	op := ops.Begin("save_config")
	defer op.End()
	return op.FailIf(s.doSaveOne(in))
}

func (s *saver) doSaveOne(in service.Message) error {
	bytes, err := yaml.Marshal(in)
	if err != nil {
		return fmt.Errorf("Unable to marshal config yaml: %v", err)
	}

	outfile, err := os.OpenFile(s.fullPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("Unable to open file %v for writing: %v", s.fullPath, err)
	}
	defer outfile.Close()

	var out io.Writer = outfile
	if s.obfuscate {
		out = rot13.NewWriter(outfile)
	}
	_, err = out.Write(bytes)
	if err != nil {
		return fmt.Errorf("Unable to write yaml to file %v: %v", s.fullPath, err)
	}
	log.Debugf("Wrote file at %v", s.fullPath)
	return nil
}
