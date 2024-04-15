package client

import (
	"github.com/getlantern/flashlight/v7/bandit"
	"github.com/getlantern/flashlight/v7/config"
)

type Option func(*Client)

// WithConnectionCallback sets the config callback
func WithSucceedingProxy(onSucceedingProxy func(isConnected bool)) Option {
	return func(client *Client) {
		client.onSucceedingProxy = onSucceedingProxy
	}
}

// WithConfig sets the config callback
func WithConfig(onConfigUpdate func(*config.Global, config.Source)) Option {
	return func(client *Client) {
		client.onConfigUpdate = onConfigUpdate
	}
}

// WithProxies sets the callback when new proxies are received
func WithProxies(onProxiesUpdate func([]bandit.Dialer, config.Source)) Option {
	return func(client *Client) {
		client.onProxiesUpdate = onProxiesUpdate
	}
}

func WithIsPro(isPro func() bool) Option {
	return func(client *Client) {
		client.isPro = isPro
	}
}
