package client

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"
)

// initBalancer takes hosts from cfg.ChainedServers and it uses them to create a
// balancer. Returns the new dialers.
func (client *Client) initBalancer(proxies map[string]*chained.ChainedServerInfo) ([]balancer.Dialer, error) {
	if len(proxies) == 0 {
		return nil, fmt.Errorf("No chained servers configured, not initializing balancer")
	}

	chained.PersistSessionStates("")
	dialers := make([]balancer.Dialer, 0, len(proxies))
	groups := groupByMultipathEndpoint(proxies)
	for endpoint, group := range groups {
		if endpoint == "" {
			log.Debugf("Adding %d individual chained servers", len(group))
			for name, s := range group {
				dialer, err := chained.CreateDialer(name, s, client.user)
				if err != nil {
					log.Errorf("Unable to configure chained server %v. Received error: %v", name, err)
					continue
				}
				log.Debugf("Adding chained server: %v", dialer.JustifiedLabel())
				dialers = append(dialers, dialer)
			}
		} else {
			log.Debugf("Adding %d chained servers for multipath endpoint %s", len(group), endpoint)
			dialer, err := chained.CreateMPDialer(endpoint, group, client.user)
			if err != nil {
				log.Errorf("Unable to configure multipath server to %v. Received error: %v", endpoint, err)
				continue
			}
			dialers = append(dialers, dialer)
		}
	}
	client.bal.Reset(dialers)

	go func() {
		for hasSucceeding := range client.bal.HasSucceedingDialer {
			client.statsTracker.SetHasSucceedingProxy(hasSucceeding)
		}
	}()

	return dialers, nil
}

type pingProxiesConf struct {
	enabled  func() bool
	interval int64
}

// ConfigurePingProxies configure the interval to ping proxies. The actual
// time to sleep varies among iterations but keeps in the range of interval +/-
// 50%. Pass 0 to pause pinging.
func (client *Client) ConfigurePingProxies(enabled func() bool, interval time.Duration) {
	if interval == 0 {
		interval = forever
	}
	client.chPingProxiesConf <- pingProxiesConf{enabled, int64(interval)}
}

func (client *Client) pingProxiesLoop() {
	var conf pingProxiesConf
	t := time.NewTimer(forever)
	resetTimer := func() {
		next := time.Duration(conf.interval/2 + rand.Int63n(conf.interval))
		t.Reset(next)
	}
	for {
		select {
		case <-t.C:
			if conf.enabled() {
				client.bal.PingProxies()
			}
			resetTimer()
		case conf = <-client.chPingProxiesConf:
			if !t.Stop() {
				<-t.C
			}
			resetTimer()
		}
	}
}

func groupByMultipathEndpoint(proxies map[string]*chained.ChainedServerInfo) map[string]map[string]*chained.ChainedServerInfo {
	groups := make(map[string]map[string]*chained.ChainedServerInfo)
	for name, s := range proxies {
		group, exists := groups[s.MultipathEndpoint]
		if !exists {
			group = make(map[string]*chained.ChainedServerInfo)
			groups[s.MultipathEndpoint] = group
		}
		group[name] = s
	}
	return groups
}
