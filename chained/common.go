// Package chained provides a chained proxy that can proxy any tcp traffic over
// any underlying transport through a remote proxy. The downstream (client) side
// of the chained setup is just a dial function. The upstream (server) side is
// just an http.Handler. The client tells the server where to connect using an
// HTTP CONNECT request.
package chained

import (
	"github.com/getlantern/common/config"
	"github.com/getlantern/golog"
	"google.golang.org/protobuf/proto"
)

var log = golog.LoggerFor("chained")

// CopyConfigs makes a safe copy of the config to avoid any data corruption for other users of the config.
func CopyConfigs(proxies map[string]*config.ProxyConfig) map[string]*config.ProxyConfig {
	proxiesCopy := make(map[string]*config.ProxyConfig)

	for k, v := range proxies {
		proxiesCopy[k] = CopyConfig(v)
	}
	return proxiesCopy
}

// CopyConfig makes a safe copy of the config to avoid any data corruption for other users of the config.
func CopyConfig(pc *config.ProxyConfig) *config.ProxyConfig {
	return proto.Clone(pc).(*config.ProxyConfig)
}
