package client

import (
	"fmt"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"
)

// HasSucceedingProxy returns a channel on which one can receive updates for the
// connected status of the balancer (i.e. does it have at least one succeeding)
// dialer.
func (client *Client) HasSucceedingProxy() <-chan bool {
	return client.bal.HasSucceedingDialer
}

// initBalancer takes hosts from cfg.ChainedServers and it uses them to create a
// balancer.
func (client *Client) initBalancer(proxies map[string]*chained.ChainedServerInfo, deviceID string) error {
	if len(proxies) == 0 {
		return fmt.Errorf("No chained servers configured, not initializing balancer")
	}

	// The dialers slice must be large enough to handle all chained and obfs4
	// servers.
	dialers := make([]balancer.Dialer, 0, len(proxies))

	// Add chained (CONNECT proxy) servers.
	log.Debugf("Adding %d chained servers", len(proxies))
	for name, s := range proxies {
		if s.PluggableTransport == "obfs4-tcp" {
			// Ignore obfs4-tcp as these are already included as plain obfs4
			continue
		}
		dialer, err := ChainedDialer(name, s, deviceID, client.proTokenGetter)
		if err != nil {
			log.Errorf("Unable to configure chained server %v. Received error: %v", name, err)
			continue
		}
		log.Debugf("Adding chained server: %v", s.Addr)
		dialers = append(dialers, dialer)
	}

	chained.TrackStatsFor(dialers)
	client.bal.Reset(dialers...)

	return nil
}
