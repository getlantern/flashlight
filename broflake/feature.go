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

const ()

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

		bo := clientcore.NewDefaultBroflakeOptions()
		if options.CTableSize != 0 {
			bo.CTableSize = options.CTableSize
		}
		if options.PTableSize != 0 {
			bo.PTableSize = options.PTableSize
		}
		if options.BusBufferSz != 0 {
			bo.BusBufferSz = options.BusBufferSz
		}
		if options.Netstated != "" {
			bo.Netstated = options.Netstated
		}

		wo := clientcore.NewDefaultWebRTCOptions()
		wo.STUNBatch = newRandomSTUNs(options.STUNSrvs)
		if options.DiscoverySrv != "" {
			wo.DiscoverySrv = options.DiscoverySrv
		}
		if options.Endpoint != "" {
			wo.Endpoint = options.Endpoint
		}
		if options.STUNBatchSize != 0 {
			wo.STUNBatchSize = options.STUNBatchSize
		}
		if options.GenesisAddr != "" {
			wo.GenesisAddr = options.GenesisAddr
		}
		if options.NATFailTimeout != 0 {
			wo.NATFailTimeout = options.NATFailTimeout
		}
		if options.ICEFailTimeout != 0 {
			wo.ICEFailTimeout = options.ICEFailTimeout
		}
		if options.Tag != "" {
			wo.Tag = options.Tag
		}

		qo := &clientcore.QUICLayerOptions{
			ServerName:         options.EgressServerName,
			InsecureSkipVerify: options.EgressInsecureSkipVerify,
		}

		InitAndStartBroflakeCensoredPeer(&Options{
			BroflakeOptions:  bo,
			WebRTCOptions:    wo,
			QUICLayerOptions: qo,
		})

		if err := proxied.EnableComponent(proxied.FlowComponentID_Broflake, NewRoundTripper()); err != nil {
			log.Errorf("Failed to enable broflake via proxied.EnableComponent: %v", proxied.FlowComponentID_Broflake, err)
		}
	})
}
