package services

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/getlantern/detour"
	"github.com/getlantern/golog"
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
}

type configService struct {
	opts          *ConfigOptions
	clientInfo    *ClientInfo
	configHandler ConfigHandler
	lastFetched   time.Time

	logger golog.Logger

	done    chan struct{}
	running bool
	mu      sync.Mutex
}

// ConfigHandler handles updating and retrieving the client config.
type ConfigHandler interface {
	// GetConfig returns the current client config.
	GetConfig() *ClientConfig
	// SetConfig sets the client config to the given config.
	SetConfig(new *ClientConfig)
}

var (
	// initialize variable so we don't have to lock mutex and check if it's nil every time someone
	// calls GetClientConfig
	_configService = &configService{}
)

// StartConfigService starts a new config service with the given options and returns a func to stop
// it. It will return an error if opts.OriginURL, opts.Rt, opts.Fetcher, or opts.OnConfig are nil.
func StartConfigService(handler ConfigHandler, opts *ConfigOptions) (StopFn, error) {
	_configService.mu.Lock()
	defer _configService.mu.Unlock()

	if _configService.running {
		return _configService.Stop, nil
	}

	switch {
	case handler == nil:
		return nil, errors.New("ConfigHandler is required")
	case opts == nil:
		return nil, errors.New("ConfigOptions is required")
	case opts.RoundTripper == nil:
		return nil, errors.New("RoundTripper is required")
	case opts.OriginURL == "":
		return nil, errors.New("OriginURL is required")
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

	config := handler.GetConfig()
	_configService.clientInfo.Country = config.GetCountry()

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

func (cs *configService) updateClientInfo(conf *ClientConfig) {
	cs.clientInfo.ProToken = conf.ProToken
	cs.clientInfo.Country = conf.Country
	cs.clientInfo.Ip = conf.Ip
}

func (cs *configService) Stop() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

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
	curConf := cs.configHandler.GetConfig()
	if curConf != nil && !configIsNew(curConf, newConf) {
		op.Set("config_changed", false)
		cs.logger.Debug("Config is unchanged")
		return sleep, nil
	}

	op.Set("config_changed", true)

	cs.updateClientInfo(newConf)
	cs.configHandler.SetConfig(newConf)

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

// newRequest returns a new ConfigRequest with the current client info, proxy ids, and the last
// time the config
func (cs *configService) newRequest() *ConfigRequest {
	conf := cs.configHandler.GetConfig()
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

// configIsNew returns true if country, proToken, or ip in currInfo differ from new or if new has
// proxy configs.
func configIsNew(cur, new *ClientConfig) bool {
	return cur.GetCountry() != new.GetCountry() ||
		cur.GetProToken() != new.GetProToken() ||
		len(new.GetProxy().GetProxies()) > 0
}
