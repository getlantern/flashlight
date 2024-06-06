package flashlight

import (
	"github.com/getlantern/flashlight/v7/bandit"
	"github.com/getlantern/flashlight/v7/config"
)

type Option func(*Flashlight)

// WithOnSucceedingProxy sets the onSucceedingProxy callback
func WithOnSucceedingProxy(onSucceedingProxy func()) Option {
	return func(client *Flashlight) {
		client.callbacks.onSucceedingProxy = onSucceedingProxy
	}
}

// WithOnConfig sets the callback that is called when a config is successfully fetched
func WithOnConfig(onConfigUpdate func(*config.Global, config.Source)) Option {
	return func(client *Flashlight) {
		client.callbacks.onConfigUpdate = onConfigUpdate
	}
}

// WithInit sets the callback that is called when flashlight is ready and has a config
// or needs to be initialized
func WithInit(onInit func()) Option {
	return func(client *Flashlight) {
		client.callbacks.onInit = onInit
	}
}

// WithOnProxies sets the callback when new proxies are received
func WithOnProxies(onProxiesUpdate func([]bandit.Dialer, config.Source)) Option {
	return func(client *Flashlight) {
		client.callbacks.onProxiesUpdate = onProxiesUpdate
	}
}
