package client

import (
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
)

// ChainedDialer creates a *balancer.Dialer backed by a chained server.
func ChainedDialer(name string, si *chained.ChainedServerInfo, uc common.UserConfig) (balancer.Dialer, error) {
	// Copy server info to allow modifying
	sic := &chained.ChainedServerInfo{}
	*sic = *si
	// Backwards-compatibility for clients that still have old obfs4
	// configurations on disk.
	if sic.PluggableTransport == "obfs4-tcp" {
		sic.PluggableTransport = "obfs4"
	}

	return chained.CreateDialer(name, sic, uc)
}
