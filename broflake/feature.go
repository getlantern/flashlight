package broflake

import (
	"net/http"
	"sync"

	"github.com/getlantern/broflake/clientcore"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/golog"
)

var (
	// TODO (nelson): I copied this logger from the previous attempt at a PR, but I don't really know
	// what it's doing. Broflake itself uses Go's default log package. Do we have any worries here?
	log = golog.LoggerFor("flashlight.broflake")

	startBroflakeOnce sync.Once

	// TODO (nelson): A successfully constructed Broflake client provides an interface to Flashlight
	// as an http.Transport. We keep the reference to said transport here. Currently, Flashlight's
	// Broflake client can only be constructed once, so we're not worried about races on this var
	// after our first call to StartBroflakeClient. However, I'm unclear on whether I need to worry
	// about Flashlight trying to dial using this transport before our Broflake client has been
	// constructed. Would this be a case to use our eventual package? Also: there's the question of
	// whether the reference to Broflake's transport should instead be kept on Flashlight's
	// client.Client. It's currently difficult to do that without creating a circular dependency?
	T *http.Transport
)

// StartBroflakeClient constructs, initializes, and starts a Broflake client which is configured to
// behave in the role of censored peer (that is, a "desktop" clientType, in Broflake parlance).
// It's idempotent (irrespective of arguments), so calling this function more than once has no effect.
func StartBroflakeClient(options *config.BroflakeOptions) {
	startBroflakeOnce.Do(func() {
		log.Debugf("Attempting to init and start a Broflake client...")

		// Create a BroflakeOptions struct, overriding defaults with values from the global config
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

		// Create a WebRTCOptions struct, overriding defaults with values from the global config
		wo := clientcore.NewDefaultWebRTCOptions()

		// If there are some STUN servers included where we expect them in the global config, we'll
		// inject a custom STUNBatch function that uses those servers. Otherwise, we'll fall back to
		// Broflake's default STUNBatch function.
		if len(options.STUNSrvs) > 0 {
			wo.STUNBatch = newRandomSTUNs(options.STUNSrvs)
		}

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

		// Construct and start a Broflake client!
		transport, err := initAndStartBroflake(bo, wo, qo)
		if err != nil {
			log.Errorf("Failed to init and start Broflake: %v", err)
			return
		}

		T = transport
	})
}

// initAndStartBroflake is a helper which abstracts the sequence of Broflake API calls required to
// construct and start an appropriate Broflake client
func initAndStartBroflake(
	bo *clientcore.BroflakeOptions,
	wo *clientcore.WebRTCOptions,
	qo *clientcore.QUICLayerOptions,
) (*http.Transport, error) {
	bfconn, _, err := clientcore.NewBroflake(bo, wo, nil)
	if err != nil {
		return nil, err
	}

	ql, err := clientcore.NewQUICLayer(bfconn, qo)
	if err != nil {
		return nil, err
	}

	return clientcore.CreateHTTPTransport(ql), nil
}
