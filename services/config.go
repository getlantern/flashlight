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
	defaultSaveDir  = ""
	defaultFilename = "proxies.conf"

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

	// Rt provides the RoundTripper the fetcher should use, which allows us to
	// dictate whether the fetcher will use dual fetching (from fronted and
	// chained URLs) or not.
	Rt http.RoundTripper

	// PollInterval specifies how frequently to poll for new config.
	PollInterval time.Duration
	// PollJitter specifies the max amount of jitter to add to the poll interval.
	PollJitter time.Duration

	// OnConfig is a callback that is called when a new config is received.
	OnConfig func(conf *ClientConfig)
}

type ConfigService struct {
	opts         *ConfigOptions
	clientInfo   *ClientInfo
	clientConfig atomic.Value
	lastFetched  time.Time

	done   chan struct{}
	once   sync.Once
	logger golog.Logger
}

// StartConfigService starts a new config service with the given options. It will return an error
// if opts.OriginURL, opts.Rt, opts.Fetcher, or opts.OnConfig are nil.
func StartConfigService(opts *ConfigOptions) (*ConfigService, error) {
	switch {
	case opts.Rt == nil:
		return nil, errors.New("RoundTripper is required")
	case opts.OnConfig == nil:
		return nil, errors.New("OnConfig is required")
	case opts.OriginURL == "":
		return nil, errors.New("OriginURL is required")
	}

	if opts.SaveDir == "" {
		opts.SaveDir = defaultSaveDir
		opts.filePath = filepath.Join(opts.SaveDir, defaultFilename)
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
	ch := &ConfigService{
		opts: opts,
		clientInfo: &ClientInfo{
			FlashlightVersion: common.LibraryVersion,
			ClientVersion:     common.CompileTimeApplicationVersion,
			UserId:            userId,
			ProToken:          opts.UserConfig.GetToken(),
		},
		clientConfig: atomic.Value{},
		done:         make(chan struct{}),
		once:         sync.Once{},
		logger:       logger,
	}

	if err := ch.init(); err != nil {
		return nil, err
	}

	ch.logger.Debug("Starting config service")
	if opts.Sticky {
		return ch, nil
	}

	fn := func() int64 {
		sleep, _ := ch.fetchConfig()
		return sleep
	}
	go callRandomly(fn, ch.opts.PollInterval, ch.opts.PollJitter, ch.done, ch.logger)
	return ch, nil
}

func (ch *ConfigService) init() error {
	ch.logger.Debug("Initializing config service")
	conf, err := readExistingClientConfig(ch.opts.filePath, ch.opts.Obfuscate)
	if conf == nil {
		if err != nil {
			ch.logger.Errorf("could not read existing config: %v", err)
		}

		ch.clientConfig.Store(&ClientConfig{})
		return err
	}

	ch.logger.Debugf("loaded saved config at %v", ch.opts.filePath)

	ch.clientInfo.Country = conf.Country
	ch.clientConfig.Store(conf)
	ch.opts.OnConfig(conf)
	return nil
}

func (ch *ConfigService) updateClientInfo(conf *ClientConfig) {
	ch.clientInfo.ProToken = conf.ProToken
	ch.clientInfo.Country = conf.Country
	ch.clientInfo.Ip = conf.Ip
}

func (ch *ConfigService) Stop() {
	ch.once.Do(func() {
		close(ch.done)
	})
}

// fetchConfig fetches the current config from the server and updates the client's config if a change
// has occurred. It returns the extra sleep time received from the server response and any error that
// occurred.
func (ch *ConfigService) fetchConfig() (int64, error) {
	op := ops.Begin("Fetching config")
	defer op.End()

	newConf, sleep, err := ch.fetch()
	if err != nil {
		return 0, op.FailIf(err)
	}

	ch.lastFetched = time.Now()

	ch.logger.Debug("Received config")
	if !configIsNew(ch.clientInfo, newConf) {
		op.Set("config_changed", false)
		ch.logger.Debug("Config is unchanged")
		return sleep, nil
	}

	op.Set("config_changed", true)

	err = saveClientConfig(ch.opts.filePath, newConf, ch.opts.Obfuscate)
	if err != nil {
		ch.logger.Error(err)
	} else {
		ch.logger.Debugf("Wrote config to %v", ch.opts.filePath)
	}

	ch.updateClientInfo(newConf)
	ch.clientConfig.Store(newConf)
	ch.opts.OnConfig(newConf)

	return sleep, nil
}

func (ch *ConfigService) fetch() (*ClientConfig, int64, error) {
	confReq := ch.newRequest()
	buf, err := proto.Marshal(confReq)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to marshal config request: %w", err)
	}

	resp, sleep, err := post(
		ch.opts.OriginURL,
		bytes.NewReader(buf),
		ch.opts.Rt,
		ch.opts.UserConfig,
		ch.logger,
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

func (ch *ConfigService) newRequest() *ConfigRequest {
	proxies := ch.Proxies()
	ids := make([]string, len(proxies))
	for _, proxy := range proxies {
		ids = append(ids, proxy.GetTrack())
	}

	confReq := &ConfigRequest{
		ClientInfo: ch.clientInfo,
		Proxy: &ConfigProxies{
			Ids:         ids,
			LastRequest: timestamppb.New(ch.lastFetched),
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

// saveClientConfig writes conf to a file at the specified path, filePath, obfuscating them if
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
func configIsNew(currInfo *ClientInfo, new *ClientConfig) bool {
	return currInfo.GetCountry() != new.GetCountry() ||
		currInfo.GetProToken() != new.GetProToken() ||
		currInfo.GetIp() != new.GetIp() ||
		len(new.GetProxy().GetProxies()) > 0
}

func (ch *ConfigService) Country() string {
	return ch.clientConfig.Load().(*ClientConfig).GetCountry()
}

func (ch *ConfigService) Ip() string {
	return ch.clientConfig.Load().(*ClientConfig).GetIp()
}

func (ch *ConfigService) ProToken() string {
	return ch.clientConfig.Load().(*ClientConfig).GetProToken()
}

func (ch *ConfigService) Proxies() []*ProxyConnectConfig {
	return ch.clientConfig.Load().(*ClientConfig).GetProxy().GetProxies()
}
