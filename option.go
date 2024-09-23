package flashlight

import (
	"github.com/getlantern/flashlight/v7/chained"
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

// WithOnDialError sets the callback that is called when an error occurs dialing a proxy. It includes the error itself and whether or not we
// have any successful dialers
func WithOnDialError(onDialError func(error, bool)) Option {
	return func(client *Flashlight) {
		client.callbacks.onDialError = onDialError
	}
}

// WithInit sets the callback that is called when flashlight is ready and has a config or needs to be initialized
func WithInit(onInit func()) Option {
	return func(client *Flashlight) {
		client.callbacks.onInit = onInit
	}
}

// WithOnProxies sets the callback when new proxies are received
func WithOnProxies(onProxiesUpdate func([]chained.Dialer, config.Source)) Option {
	return func(client *Flashlight) {
		client.callbacks.onProxiesUpdate = onProxiesUpdate
	}
}
