package client

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/getlantern/flashlight/balancer"
)

var (
	bal = balancer.New(&balancer.Opts{
		Strategy: balancer.QualityFirst,
	})
)

// initBalancer takes hosts from cfg.ChainedServers and it uses them to create a
// balancer.
func (client *Client) initBalancer(proxies map[string]*ChainedServerInfo, deviceID string) error {
	if len(proxies) == 0 {
		return fmt.Errorf("No chained servers configured, not initializing balancer")
	}
	// The dialers slice must be large enough to handle all chained and obfs4
	// servers.
	dialers := make([]*balancer.Dialer, 0, len(proxies))

	// Add chained (CONNECT proxy) servers.
	log.Debugf("Adding %d chained servers", len(proxies))
	for name, s := range proxies {
		if strings.HasSuffix(s.PluggableTransport, "tcp") && runtime.GOOS == "android" {
			log.Debugf("Ignoring non-KCP on android for now.")
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

	bal.Reset(dialers...)
	return nil
}
