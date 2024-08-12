package services

import (
	"bytes"
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

	"github.com/getlantern/flashlight/v7/apipb"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/ops"
)

const (
	defaultConfigSaveDir  = ""
	defaultConfigFilename = "proxies.conf"

	defaultConfigPollInterval = 3 * time.Minute
	defaultConfigPollJitter   = 2 * time.Minute
)

// ConfigOptions specifies the options to use for ConfigService.
type ConfigOptions struct {
	// URL to use for fetching this config.
	OriginURL string

	// UserConfig contains data for communicating the user details to upstream
	// servers in HTTP headers, such as the pro token and other options.
	UserConfig common.UserConfig

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
	clientInfo    *apipb.ConfigRequest_ClientInfo
	configHandler ConfigHandler
	lastFetched   time.Time

	sender *sender

	done    chan struct{}
	running bool
	// runningMu is used to protect the running field.
	runningMu sync.Mutex
}

// ConfigHandler handles updating and retrieving the client config.
type ConfigHandler interface {
	// GetConfig returns the current client config.
	GetConfig() *apipb.ConfigResponse
	// SetConfig sets the client config to the given config.
	SetConfig(new *apipb.ConfigResponse)
}

var (
	_configService = &configService{sender: &sender{}}
)

// StartConfigService starts a new config service with the given options and returns a func to stop
// it. It will return an error if opts.OriginURL, opts.Rt, or opts.OnConfig are nil.
func StartConfigService(handler ConfigHandler, opts *ConfigOptions) (StopFn, error) {
	_configService.runningMu.Lock()
	defer _configService.runningMu.Unlock()

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
	_configService.clientInfo = &apipb.ConfigRequest_ClientInfo{
		FlashlightVersion: common.LibraryVersion,
		ClientVersion:     common.CompileTimeApplicationVersion,
		UserId:            userId,
		ProToken:          opts.UserConfig.GetToken(),
	}
	_configService.done = make(chan struct{})

	config := handler.GetConfig()
	_configService.clientInfo.Country = config.GetCountry()

	logger.Debug("Starting config service")
	_configService.running = true

	fn := func() int64 {
		sleep, _ := _configService.fetchConfig()
		return sleep
	}
	go callRandomly(fn, opts.PollInterval, opts.PollJitter, _configService.done)

	return _configService.Stop, nil
}

func (cs *configService) Stop() {
	cs.runningMu.Lock()
	defer cs.runningMu.Unlock()

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
		logger.Errorf("configservice: Failed to fetch config: %v", err)
		return 0, op.FailIf(err)
	}

	cs.lastFetched = time.Now()

	logger.Debug("configservice: Received config")
	curConf := cs.configHandler.GetConfig()
	if curConf != nil && !configIsNew(newConf) {
		op.Set("config_changed", false)
		logger.Debug("configservice: Config is unchanged")
		return sleep, nil
	}

	op.Set("config_changed", true)

	cs.clientInfo.ProToken = newConf.ProToken
	cs.clientInfo.Country = newConf.Country
	cs.clientInfo.Ip = newConf.Ip

	cs.configHandler.SetConfig(newConf)

	return sleep, nil
}

func (cs *configService) fetch() (*apipb.ConfigResponse, int64, error) {
	confReq := cs.newRequest()
	buf, err := proto.Marshal(confReq)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to marshal config request: %w", err)
	}

	resp, sleep, err := cs.sender.post(
		cs.opts.OriginURL,
		bytes.NewReader(buf),
		cs.opts.RoundTripper,
		cs.opts.UserConfig,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("config request failed: %w", err)
	}
	defer resp.Close()

	configBytes, err := io.ReadAll(resp)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to read config response: %w", err)
	}

	newConf := &apipb.ConfigResponse{}
	if err = proto.Unmarshal(configBytes, newConf); err != nil {
		return nil, 0, fmt.Errorf("unable to unmarshal config: %w", err)
	}

	return newConf, sleep, err
}

// newRequest returns a new ConfigRequest with the current client info, proxy ids, and the last
// time the config was fetched.
func (cs *configService) newRequest() *apipb.ConfigRequest {
	conf := cs.configHandler.GetConfig()
	proxies := []*apipb.ProxyConnectConfig{}
	if conf != nil { // not the first request
		proxies = conf.GetProxy().GetProxies()
	}

	ids := make([]string, len(proxies))
	for _, proxy := range proxies {
		ids = append(ids, proxy.Name)
	}

	confReq := &apipb.ConfigRequest{
		ClientInfo: cs.clientInfo,
		Proxy: &apipb.ConfigRequest_Proxy{
			Ids:         ids,
			LastRequest: timestamppb.New(cs.lastFetched),
		},
	}

	return confReq
}

// configIsNew returns true if any fields contain values.
func configIsNew(new *apipb.ConfigResponse) bool {
	// We only need to check if the fields we're interested in contain values because the server
	// will only send us new values if they have changed.
	return new.Country != "" || new.ProToken != "" || len(new.Proxy.Proxies) > 0
}
