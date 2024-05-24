package client

import (
	"github.com/getlantern/flashlight/v7/bandit"
	"github.com/getlantern/flashlight/v7/config"
)

type Option func(*Client)

// WithSucceedingProxy sets the onSucceedingProxy callback
func WithSucceedingProxy(onSucceedingProxy func(isConnected bool)) Option {
	return func(client *Client) {
		client.callbacks.onSucceedingProxy = onSucceedingProxy
	}
}

// WithOnConfig sets the callback that is called when a config is successfully fetched
func WithOnConfig(onConfigUpdate func(*config.Global, config.Source)) Option {
	return func(client *Client) {
		client.callbacks.onConfigUpdate = onConfigUpdate
	}
}

// WithDetour sets the callback that determines whether or not to use detour
func WithDetour(useDetour func() bool) Option {
	return func(client *Client) {
		client.callbacks.useDetour = useDetour
	}
}

// WithInit sets the callback that is called when flashlight is ready and has a config
// or needs to be initialized
func WithInit(onInit func()) Option {
	return func(client *Client) {
		client.callbacks.onInit = onInit
	}
}

// WithShortcut sets the callback that determines whether or not to use shortcut
func WithShortcut(useShortcut func() bool) Option {
	return func(client *Client) {
		client.callbacks.useShortcut = useShortcut
	}
}

// WithProxies sets the callback when new proxies are received
func WithProxies(onProxiesUpdate func([]bandit.Dialer, config.Source)) Option {
	return func(client *Client) {
		client.callbacks.onProxiesUpdate = onProxiesUpdate
	}
}

// WithIsPro sets the callback that checks whether or not a user is Pro
func WithIsPro(isPro func() bool) Option {
	return func(client *Client) {
		client.isPro = isPro
	}
}
