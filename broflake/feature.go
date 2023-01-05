package broflake

import (
	"sync"

	clientcore "github.com/getlantern/broflake/clientcore"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/proxied"
)

var (
	startBroflakeOnce sync.Once
)

// startBroflakeCensoredPeerIfNecessary initializes broflake censored
// peer and related uses in flashlight if the broflake feature is enabled.
func StartBroflakeCensoredPeerIfNecessary(enabled bool, options *config.BroflakeOptions) {
	if !enabled {
		log.Debugf("not enabling broflake features...")
		return
	}

	log.Debugf("attempting to enable broflake features...")

	startBroflakeOnce.Do(func() {
		log.Debugf("really attempting to enable broflake features once...")

		opt := &clientcore.WebRTCOptions{
			DiscoverySrv:   options.DiscoverySrv,
			Endpoint:       options.Endpoint,
			GenesisAddr:    options.GenesisAddr,
			NATFailTimeout: options.NATFailTimeout,
			ICEFailTimeout: options.ICEFailTimeout,
			STUNBatch:      RandomSTUNs(options.STUNSrvs),
			STUNBatchSize:  options.STUNBatchSize,
		}

		InitAndStartBroflakeCensoredPeer(opt)

		if err := proxied.EnableComponent(proxied.FlowComponentID_Broflake, NewRoundTripper()); err != nil {
			log.Errorf("Failed to enable broflake via proxied.EnableComponent: %v", proxied.FlowComponentID_Broflake, err)
		}
	})
}
