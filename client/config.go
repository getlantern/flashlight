package client

import (
	"fmt"
	"path/filepath"

	commonconfig "github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/v7/config"
	"github.com/getlantern/flashlight/v7/domainrouting"
	"github.com/getlantern/flashlight/v7/otel"
	"github.com/getlantern/flashlight/v7/proxied"
	"github.com/getlantern/fronted"
)

// HandledErrorType is used to differentiate error types to handlers configured via
// Flashlight.SetErrorHandler.
type HandledErrorType int

const (
	ErrorTypeProxySaveFailure  HandledErrorType = iota
	ErrorTypeConfigSaveFailure HandledErrorType = iota
)

func (t HandledErrorType) String() string {
	switch t {
	case ErrorTypeProxySaveFailure:
		return "proxy save failure"
	case ErrorTypeConfigSaveFailure:
		return "config save failure"
	default:
		return fmt.Sprintf("unrecognized error type %d", t)
	}
}

func (client *Client) onGlobalConfig(cfg *config.Global, src config.Source) {
	log.Debugf("Got global config from %v", src)
	client.mxGlobal.Lock()
	client.global = cfg
	client.mxGlobal.Unlock()
	domainrouting.Configure(cfg.DomainRoutingRules, cfg.ProxiedSites)
	client.applyClientConfig(cfg)
	client.applyOtel(cfg)
	client.onConfigUpdate(cfg, src)
	if client.onReady != nil {
		client.onReady(true)
	}
}

func (client *Client) applyClientConfig(cfg *config.Global) {
	client.DNSResolutionMapForDirectDialsEventual.Set(cfg.Client.DNSResolutionMapForDirectDials)
	certs, err := cfg.TrustedCACerts()
	if err != nil {
		log.Errorf("Unable to get trusted ca certs, not configuring fronted: %s", err)
	} else if cfg.Client != nil && cfg.Client.Fronted != nil {
		fronted.Configure(certs, cfg.Client.FrontedProviders(), config.DefaultFrontedProviderID,
			filepath.Join(client.configDir, "masquerade_cache"))
	} else {
		log.Errorf("Unable to configured fronted (no config)")
	}
}

// EnableNamedDomainRules adds named domain rules specified as arguments to the domainrouting rules table
func (client *Client) EnableNamedDomainRules(names ...string) {
	client.mxGlobal.RLock()
	global := client.global
	client.mxGlobal.RUnlock()
	if global == nil {
		return
	}
	for _, name := range names {
		if v, ok := global.NamedDomainRoutingRules[name]; ok {
			if err := domainrouting.AddRules(v); err != nil {
				_ = log.Errorf("Unable to add named domain routing rules: %v", err)
			}
		} else {
			log.Debugf("Named domain routing rule %s is not defined in global config", name)
		}
	}
}

// DisableNamedDomainRules removes named domain rules specified as arguments from the domainrouting rules table
func (client *Client) DisableNamedDomainRules(names ...string) {
	client.mxGlobal.RLock()
	global := client.global
	client.mxGlobal.RUnlock()
	if global == nil {
		return
	}
	for _, name := range names {
		if v, ok := global.NamedDomainRoutingRules[name]; !ok {
			if err := domainrouting.RemoveRules(v); err != nil {
				_ = log.Errorf("Unable to remove named domain routing rules: %v", err)
			}
		} else {
			log.Debugf("Named domain routing rule %s is not defined in global config", name)
		}
	}
}

func (client *Client) applyOtel(cfg *config.Global) {
	if cfg.Otel != nil && client.featureEnabled(config.FeatureOtel) {
		otel.Configure(cfg.Otel)
	}
}

// SetErrorHandler configures error handling. All errors provided to the handler are significant,
// but not enough to stop operation of the Flashlight instance. This method must be called before
// calling Run. All errors provided to the handler will be of a HandledErrorType defined in this
// package. The handler may be called multiple times concurrently.
//
// If no handler is configured, these errors will be logged on the ERROR level.
func (client *Client) SetErrorHandler(handler func(t HandledErrorType, err error)) {
	if handler == nil {
		return
	}
	client.errorHandler = handler
}

func (client *Client) startConfigFetch() func() {
	proxiesDispatch := func(conf interface{}, src config.Source) {
		proxyMap := conf.(map[string]*commonconfig.ProxyConfig)
		client.notifyProxyListeners(proxyMap, src)
	}
	globalDispatch := func(conf interface{}, src config.Source) {
		cfg := conf.(*config.Global)
		log.Debugf("Applying global config")
		client.onGlobalConfig(cfg, src)
	}
	rt := proxied.ParallelPreferChained()

	onProxiesSaveError := func(err error) {
		client.errorHandler(ErrorTypeProxySaveFailure, err)
	}
	onConfigSaveError := func(err error) {
		client.errorHandler(ErrorTypeConfigSaveFailure, err)
	}

	stopConfig := config.Init(
		client.configDir, client.flagsAsMap, client.user,
		proxiesDispatch, onProxiesSaveError,
		globalDispatch, onConfigSaveError, rt)
	return stopConfig
}
