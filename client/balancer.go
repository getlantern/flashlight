package client

import (
	"fmt"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"

	log "github.com/sirupsen/logrus"
)

// initBalancer takes hosts from cfg.ChainedServers and it uses them to create a
// balancer.
func (client *Client) initBalancer(proxies map[string]*chained.ChainedServerInfo) error {
	if len(proxies) == 0 {
		return fmt.Errorf("No chained servers configured, not initializing balancer")
	}

	log.Debugf("Adding %d chained servers", len(proxies))
	dialers := make([]balancer.Dialer, 0, len(proxies))
	for name, s := range proxies {
		if s.PluggableTransport == "obfs4-tcp" {
			log.Debugf("Ignoring obfs4-tcp server: %v", name)
			// Ignore obfs4-tcp as these are already included as plain obfs4
			continue
		}
		dialer, err := ChainedDialer(name, s, client.user)
		if err != nil {
			log.Errorf("Unable to configure chained server %v. Received error: %v", name, err)
			continue
		}
		log.Debugf("Adding chained server: %v", dialer.JustifiedLabel())
		dialers = append(dialers, dialer)
	}

	chained.TrackStatsFor(dialers)
	client.bal.Reset(dialers)

	go func() {
		for hasSucceeding := range client.bal.HasSucceedingDialer {
			client.statsTracker.SetHasSucceedingProxy(hasSucceeding)
		}
	}()

	return nil
}
