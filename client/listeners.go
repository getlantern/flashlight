package client

import (
	commonconfig "github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/v7/chained"
	"github.com/getlantern/flashlight/v7/config"
)

func (client *Client) addProxyListener(listener func(proxies map[string]*commonconfig.ProxyConfig, src config.Source)) {
	client.mxProxyListeners.Lock()
	defer client.mxProxyListeners.Unlock()
	client.proxyListeners = append(client.proxyListeners, listener)
}

func (client *Client) notifyProxyListeners(proxies map[string]*commonconfig.ProxyConfig, src config.Source) {
	client.mxProxyListeners.RLock()
	defer client.mxProxyListeners.RUnlock()
	for _, l := range client.proxyListeners {
		// Make absolutely sure we don't hit data races with different go routines
		// accessing shared data -- give each go routine it's own copy.
		proxiesCopy := chained.CopyConfigs(proxies)
		go l(proxiesCopy, src)
	}
}
