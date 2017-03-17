package client

import (
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"
)

// ChainedDialer creates a *balancer.Dialer backed by a chained server.
func ChainedDialer(name string, si *chained.ChainedServerInfo, deviceID string, proTokenGetter func() string) (balancer.Dialer, error) {
	// Copy server info to allow modifying
	sic := &chained.ChainedServerInfo{}
	*sic = *si
	// Backwards-compatibility for clients that still have old obfs4
	// configurations on disk.
	if sic.PluggableTransport == "obfs4-tcp" {
		sic.PluggableTransport = "obfs4"
	}

	return chained.CreateDialer(name, sic, deviceID, proTokenGetter)
}
