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

// WithOnConfig sets the callback that is called when a config is successfully fetched
func WithOnConfig(onConfigUpdate func(*config.Global, config.Source)) Option {
	return func(client *Client) {
		client.onConfigUpdate = onConfigUpdate
	}
}

// WithDetour sets the callback that determines whether or not to use detour
func WithDetour(useDetour func() bool) Option {
	return func(client *Client) {
		client.useDetour = useDetour
	}
}

// WithReady sets the callback that is called when flashlight is ready and has a config
// or needs to be initialized
func WithReady(onReady func(bool)) Option {
	return func(client *Client) {
		client.onReady = onReady
	}
}

// WithShortcut sets the callback that determines whether or not to use shortcut
func WithShortcut(useShortcut func() bool) Option {
	return func(client *Client) {
		client.useShortcut = useShortcut
	}
}

// WithProxies sets the callback when new proxies are received
func WithProxies(onProxiesUpdate func([]bandit.Dialer, config.Source)) Option {
	return func(client *Client) {
		client.onProxiesUpdate = onProxiesUpdate
	}
}

// WithIsPro sets the callback that checks whether or not a user is Pro
func WithIsPro(isPro func() bool) Option {
	return func(client *Client) {
		client.isPro = isPro
	}
}
