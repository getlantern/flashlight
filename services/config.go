package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/getlantern/detour"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/getlantern/flashlight/v7/apipb"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/ops"
)

const defaultConfigPollInterval = 3 * time.Minute

// ConfigOptions specifies the options to use for ConfigService.
type ConfigOptions struct {
	// URL to use for fetching this config.
	OriginURL string

	// UserConfig contains data for communicating the user details to upstream
	// servers in HTTP headers, such as the pro token and other options.
	UserConfig common.UserConfig

	// PollInterval specifies how frequently to poll for new config.
	PollInterval time.Duration
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

var _configService = &configService{sender: &sender{}}

// StartConfigService starts a new config service with the given options and returns a func to stop
// it. It will return an error if opts.OriginURL, opts.Rt, or opts.OnConfig are nil.
func StartConfigService(handler ConfigHandler, opts *ConfigOptions) (StopFn, error) {
	_configService.runningMu.Lock()
	defer _configService.runningMu.Unlock()

	if _configService.running {
		return _configService.Stop, nil
	}

	logger.Debug("Starting config service")
	switch {
	case handler == nil:
		return nil, errors.New("ConfigHandler is required")
	case opts == nil:
		return nil, errors.New("ConfigOptions is required")
	case opts.OriginURL == "":
		return nil, errors.New("OriginURL is required")
	}

	if opts.PollInterval <= 0 {
		opts.PollInterval = defaultConfigPollInterval
	}

	u, err := url.Parse(opts.OriginURL)
	if err != nil {
		logger.Errorf("configservice: Unable to parse chained cloud config URL: %v", err)
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

	_configService.configHandler = handler
	config := handler.GetConfig()
	_configService.clientInfo.Country = config.GetCountry()

	_configService.running = true

	fn := func() int64 {
		sleep, _ := _configService.fetchConfig()
		return sleep
	}
	go callRandomly("configservice", fn, opts.PollInterval, _configService.done)

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
	op := ops.Begin("Fetching_userconfig")
	defer op.End()

	newConf, sleep, err := cs.fetch()
	if err != nil {
		logger.Errorf("configservice: Failed to fetch config: %v", err)
		return 0, op.FailIf(err)
	}

	cs.lastFetched = time.Now()

	logger.Debug("configservice: Received config")
	logger.Tracef("configservice: new config:\n%v", newConf.String())
	if newConf == nil {
		op.Set("config_changed", false)
		logger.Debug("configservice: Config is unchanged")
		return sleep, nil
	}

	op.Set("config_changed", true)

	if newConf.ProToken != "" {
		cs.clientInfo.ProToken = newConf.ProToken
	}
	if newConf.Country != "" {
		cs.clientInfo.Country = newConf.Country
	}
	if newConf.Ip != "" {
		cs.clientInfo.Ip = newConf.Ip
	}
	cs.configHandler.SetConfig(newConf)
	return sleep, nil
}

func (cs *configService) fetch() (*apipb.ConfigResponse, int64, error) {
	var (
		resp  *http.Response
		sleep int64
	)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cancel()
		req, err := cs.newRequest(ctx)
		if err != nil {
			return nil, 0, err
		}

		logger.Debugf("configservice: fetching config from %v", req.URL)
		resp, sleep, err = cs.sender.post(req, common.GetHTTPClient())
		if err == nil {
			break
		}
		if resp != nil {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				logger.Errorf("configservice: could not read failed response body: %v", err)
			} else {
				logger.Errorf("configservice: failed response body: %s", body)
			}
		}
		logger.Errorf("configservice: Failed to fetch config: %v", err)
		retryWait := time.Duration(sleep) * time.Second
		logger.Debugf("configservice: Retrying in %v", retryWait)
		select {
		case <-time.After(retryWait):
		case <-cs.done:
			return nil, 0, errors.New("configservice: fetch cancelled")
		}
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil, 0, nil // no config changes
	}

	configBytes, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, 0, fmt.Errorf("unable to read config response: %w", err)
	}

	newConf := &apipb.ConfigResponse{}
	if err = proto.Unmarshal(configBytes, newConf); err != nil {
		return nil, 0, fmt.Errorf("unable to unmarshal config: %w", err)
	}

	return newConf, sleep, nil
}

// newRequest returns a new ConfigRequest with the current client info, proxy ids, and the last
// time the config was fetched.
func (cs *configService) newRequest(ctx context.Context) (*http.Request, error) {
	conf := cs.configHandler.GetConfig()
	proxies := []*apipb.ProxyConnectConfig{}
	if conf != nil { // not the first request
		proxies = conf.GetProxy().GetProxies()
	}

	names := make([]string, len(proxies))
	for i, proxy := range proxies {
		names[i] = proxy.Name
	}

	confReq := &apipb.ConfigRequest{
		ClientInfo: cs.clientInfo,
		Proxy: &apipb.ConfigRequest_Proxy{
			Names:       names,
			LastRequest: timestamppb.New(cs.lastFetched),
		},
	}

	buf, err := proto.Marshal(confReq)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal config request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cs.opts.OriginURL, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("unable to create request")
	}

	common.AddCommonHeaders(cs.opts.UserConfig, req)
	req.Header.Set("Content-Type", "application/x-protobuf")
	// Prevents intermediate nodes (domain-fronters) from caching the content
	req.Header.Set("Cache-Control", "no-cache")

	var headers string
	for k, v := range req.Header {
		headers += fmt.Sprintf("%s: %s\n", k, v)
	}
	logger.Tracef("configservice: Request:\n%v%v", headers, confReq.String())
	return req, nil
}
