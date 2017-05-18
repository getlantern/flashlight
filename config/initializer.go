package config

import (
	"time"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config/generated"
)

const (
	globalChained = "https://globalconfig.flashlightproxy.com/global.yaml.gz"
	globalFronted = "https://d24ykmup0867cj.cloudfront.net/global.yaml.gz"

	globalChainedStaging = "https://globalconfig.flashlightproxy.com/global.yaml.gz"
	globalFrontedStaging = "https://d24ykmup0867cj.cloudfront.net/global.yaml.gz"

	// Note: we keep using HTTP so the proxy / CDN can inject headers required
	// by the config-server.
	// Data is always encrypted when transfering on the wire because:
	// 1) The requests to proxy / CDN are over encrypted tunnel,
	// 2) The CDN is configured to fetch origin via HTTPS, and
	// 3) The proxy rewrites config-server requests to HTTPS.
	proxiesChained = "http://config.getiantem.org/proxies.yaml.gz"
	proxiesFronted = "http://d2wi0vwulmtn99.cloudfront.net/proxies.yaml.gz"

	proxiesChainedStaging = "http://config-staging.getiantem.org/proxies.yaml.gz"
	proxiesFrontedStaging = "http://d33pfmbpauhmvd.cloudfront.net/proxies.yaml.gz"

	globalYAML  = "global.yaml"
	proxiesYAML = "proxies.yaml"

	globalYAMLStaging  = "global-staging.yaml"
	proxiesYAMLStaging = "proxies-staging.yaml"
)

func DefaultConfigOpts() *ConfigOpts {
	opts := &ConfigOpts{
		Obfuscate: true,
		Global: FetchOpts{
			FileName:     globalYAML,
			EmbeddedName: globalYAML,
			EmbeddedData: generated.GlobalConfig,
			ChainedURL:   globalChained,
			FrontedURL:   globalFronted,
			FetchInteval: 24 * time.Hour,
		},
		Proxies: FetchOpts{
			FileName:     proxiesYAML,
			EmbeddedName: proxiesYAML,
			EmbeddedData: generated.EmbeddedProxies,
			ChainedURL:   proxiesChained,
			FrontedURL:   proxiesFronted,
			FetchInteval: 1 * time.Minute,
		},
	}
	if common.Staging {
		opts.Global.FileName = globalYAMLStaging
		opts.Global.ChainedURL = globalChainedStaging
		opts.Global.FrontedURL = globalFrontedStaging

		opts.Proxies.FileName = proxiesYAMLStaging
		opts.Proxies.ChainedURL = proxiesChainedStaging
		opts.Proxies.FrontedURL = proxiesFrontedStaging
	}
	return opts
}
