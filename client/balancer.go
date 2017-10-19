package client

import (
	"errors"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"
)

// initBalancer takes hosts from cfg.ChainedServers and it uses them to create a
// balancer.
func (client *Client) initBalancer(proxies map[string]*chained.ChainedServerInfo,
	deviceID string) ([]balancer.Dialer, error) {
	dialers := []balancer.Dialer{}
	var err error
	if len(proxies) == 0 {
		err = errors.New("No chained servers configured, not initializing balancer")
		log.Error(err)
		return dialers, err
	}

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

	go func() {
		for hasSucceeding := range client.bal.HasSucceedingDialer {
			client.statsTracker.SetHasSucceedingProxy(hasSucceeding)
		}
	}()

	return dialers, nil
}
