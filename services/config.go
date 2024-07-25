package services

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/detour"
	"github.com/getlantern/golog"
	"github.com/getlantern/rot13"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/ops"

	"github.com/getlantern/lantern-cloud/cmd/api/apipb"
)

const (
	defaultConfigSaveDir  = ""
	defaultConfigFilename = "proxies.conf"

	defaultConfigPollInterval = 3 * time.Minute
	defaultConfigPollJitter   = 2 * time.Minute
)

// aliases for better readability
type (
	ConfigRequest      = apipb.ConfigRequest
	ClientInfo         = apipb.ConfigRequest_ClientInfo
	ConfigProxies      = apipb.ConfigRequest_Proxy
	ClientConfig       = apipb.ConfigResponse
	ProxyConnectConfig = apipb.ProxyConnectConfig
)

// ConfigOptions specifies the options to use for ConfigService.
type ConfigOptions struct {
	// SaveDir is the directory where we should save new configs and also look
	// for existing saved configs.
	SaveDir  string
	filePath string

	// obfuscate specifies whether or not to obfuscate the config on disk.
	Obfuscate bool

	// Name specifies the name of the config file both on disk and in the
	// embedded config that uses tarfs (the same in the interest of using
	// configuration by convention).
	Name string

	// URL to use for fetching this config.
	OriginURL string

	// UserConfig contains data for communicating the user details to upstream
	// servers in HTTP headers, such as the pro token and other options.
	UserConfig common.UserConfig

	// Sticky specifies whether or not to only use the local config and not
	// update it with remote data.
	Sticky bool

	// RoundTripper provides the http.RoundTripper the fetcher should use, which allows us to
	// dictate whether the fetcher will use dual fetching (from fronted and chained URLs) or not.
	RoundTripper http.RoundTripper

	// PollInterval specifies how frequently to poll for new config.
	PollInterval time.Duration
	// PollJitter specifies the max amount of jitter to add to the poll interval.
	PollJitter time.Duration

	// OnConfig is a callback that is called when a new config is received.
	OnConfig func(old, new *ClientConfig)
}

type configService struct {
	opts         *ConfigOptions
	clientInfo   *ClientInfo
	clientConfig atomic.Value
	lastFetched  time.Time

	done    chan struct{}
	running bool
	logger  golog.Logger
}

var (
	// initialize variable so we don't have to lock mutex and check if it's nil every time someone
	// calls GetClientConfig
	_configService  = &configService{clientConfig: atomic.Value{}}
	configServiceMu sync.Mutex
)

// StartConfigService starts a new config service with the given options and returns a func to stop
// it. It will return an error if opts.OriginURL, opts.Rt, opts.Fetcher, or opts.OnConfig are nil.
func StartConfigService(opts *ConfigOptions) (StopFn, error) {
	configServiceMu.Lock()
	defer configServiceMu.Unlock()

	if _configService != nil && _configService.running {
		return _configService.Stop, nil
	}

	switch {
	case opts.RoundTripper == nil:
		return nil, errors.New("RoundTripper is required")
	case opts.OnConfig == nil:
		return nil, errors.New("OnConfig is required")
	case opts.OriginURL == "":
		return nil, errors.New("OriginURL is required")
	}

	if opts.SaveDir == "" {
		opts.SaveDir = defaultConfigSaveDir
		opts.filePath = filepath.Join(opts.SaveDir, defaultConfigFilename)
	}

	if opts.PollInterval <= 0 {
		opts.PollInterval = defaultConfigPollInterval
	}

	if opts.PollJitter <= 0 {
		opts.PollJitter = defaultConfigPollJitter
	}

	logger := golog.LoggerFor("configservice")

	u, err := url.Parse(opts.OriginURL)
	if err != nil {
		logger.Fatalf("Unable to parse chained cloud config URL: %v", err)
	}

	detour.ForceWhitelist(u.Host)

	userId := strconv.Itoa(int(opts.UserConfig.GetUserID()))
	_configService.opts = opts
	_configService.clientInfo = &ClientInfo{
		FlashlightVersion: common.LibraryVersion,
		ClientVersion:     common.CompileTimeApplicationVersion,
		UserId:            userId,
		ProToken:          opts.UserConfig.GetToken(),
	}
	_configService.done = make(chan struct{})
	_configService.logger = logger

	if err := _configService.init(); err != nil {
		return nil, err
	}

	_configService.logger.Debug("Starting config service")
	_configService.running = true
	if opts.Sticky {
		return _configService.Stop, nil
	}

	fn := func() int64 {
		sleep, _ := _configService.fetchConfig()
		return sleep
	}
	go callRandomly(fn, opts.PollInterval, opts.PollJitter, _configService.done, _configService.logger)

	return _configService.Stop, nil
}

func (cs *configService) init() error {
	cs.logger.Debug("Initializing config service")
	conf, err := readExistingClientConfig(cs.opts.filePath, cs.opts.Obfuscate)
	if conf == nil {
		if err != nil {
			cs.logger.Errorf("could not read existing config: %v", err)
		}

		cs.clientConfig.Store(&ClientConfig{})
		return err
	}

	cs.logger.Debugf("loaded saved config at %v", cs.opts.filePath)

	cs.clientInfo.Country = conf.Country
	cs.clientConfig.Store(conf)
	cs.opts.OnConfig(nil, conf)

	return nil
}

func (cs *configService) updateClientInfo(conf *ClientConfig) {
	cs.clientInfo.ProToken = conf.ProToken
	cs.clientInfo.Country = conf.Country
	cs.clientInfo.Ip = conf.Ip
}

func (cs *configService) Stop() {
	configServiceMu.Lock()
	defer configServiceMu.Unlock()

	if !cs.running {
		return
	}

	close(cs.done)
	cs.running = false
}

// fetchConfig fetches the current config from the server and updates the client's config if a change
// has occurred. It returns the extra sleep time received from the server response and any error that
// occurred.
func (cs *configService) fetchConfig() (int64, error) {
	op := ops.Begin("Fetching config")
	defer op.End()

	newConf, sleep, err := cs.fetch()
	if err != nil {
		return 0, op.FailIf(err)
	}

	cs.lastFetched = time.Now()

	cs.logger.Debug("Received config")
	curConf := GetClientConfig()
	if curConf != nil && !configIsNew(curConf, newConf) {
		op.Set("config_changed", false)
		cs.logger.Debug("Config is unchanged")
		return sleep, nil
	}

	op.Set("config_changed", true)

	err = saveClientConfig(cs.opts.filePath, newConf, cs.opts.Obfuscate)
	if err != nil {
		cs.logger.Error(err)
	} else {
		cs.logger.Debugf("Wrote config to %v", cs.opts.filePath)
	}

	cs.updateClientInfo(newConf)
	old := cs.clientConfig.Swap(newConf)
	cs.opts.OnConfig(old.(*ClientConfig), newConf)

	return sleep, nil
}

func (cs *configService) fetch() (*ClientConfig, int64, error) {
	confReq := cs.newRequest()
	buf, err := proto.Marshal(confReq)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to marshal config request: %w", err)
	}

	resp, sleep, err := post(
		cs.opts.OriginURL,
		bytes.NewReader(buf),
		cs.opts.RoundTripper,
		cs.opts.UserConfig,
		cs.logger,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("config request failed: %w", err)
	}
	defer resp.Close()

	gzReader, err := gzip.NewReader(resp)
	if err != nil {
		return nil, sleep, fmt.Errorf("unable to open gzip reader: %w", err)
	}

	configBytes, err := io.ReadAll(gzReader)
	gzReader.Close()

	newConf := &ClientConfig{}
	if err = proto.Unmarshal(configBytes, newConf); err != nil {
		return nil, 0, fmt.Errorf("unable to unmarshal config: %w", err)
	}

	return newConf, sleep, err
}

func (cs *configService) newRequest() *ConfigRequest {
	conf := GetClientConfig()
	proxies := []*ProxyConnectConfig{}
	if conf != nil { // not the first request
		proxies = conf.GetProxy().GetProxies()
	}

	ids := make([]string, len(proxies))
	for _, proxy := range proxies {
		ids = append(ids, proxy.GetTrack())
	}

	confReq := &ConfigRequest{
		ClientInfo: cs.clientInfo,
		Proxy: &ConfigProxies{
			Ids:         ids,
			LastRequest: timestamppb.New(cs.lastFetched),
		},
	}

	return confReq
}

// readExistingClientConfig reads a config from a file at the specified path, filePath,
// deobfuscating it if obfuscate is true.
func readExistingClientConfig(filePath string, obfuscate bool) (*ClientConfig, error) {
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

	conf := &ClientConfig{}
	err = proto.Unmarshal(bytes, conf)
	return conf, err
}

// saveClientConfig writes conf to a file at the specified path, filePath, obfuscating it if
// obfuscate is true. If the file already exists, it will be overwritten.
func saveClientConfig(filePath string, conf *ClientConfig, obfuscate bool) error {
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

// configIsNew returns true if country, proToken, or ip in currInfo differ from new or if new has
// proxy configs.
func configIsNew(cur, new *ClientConfig) bool {
	return cur.GetCountry() != new.GetCountry() ||
		cur.GetProToken() != new.GetProToken() ||
		len(new.GetProxy().GetProxies()) > 0
}

// GetClientConfig returns the current client config.
func GetClientConfig() *ClientConfig {
	// We don't need to lock the mutex here because we know that the configService var is not nil
	return _configService.clientConfig.Load().(*ClientConfig)
}

// GetCountry returns the country from the current client config. If there is no config, it returns
// the default country.
func GetCountry() string {
	conf := GetClientConfig()
	if conf == nil { // no config yet
		return ""
	}

	return conf.GetCountry()
}
