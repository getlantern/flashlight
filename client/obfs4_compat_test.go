package client

import (
	"testing"

	"github.com/getlantern/flashlight/chained"
	"github.com/stretchr/testify/assert"
)

func TestOBFS4BackwardCompatibility(t *testing.T) {
	assert.NoError(t, buildDialer(), "obfs4 protocol in old config format should have been converted to obfs4-tcp")
}

func buildDialer() error {
	s := &chained.ChainedServerInfo{
		Addr:               "1.1.1.1",
		AuthToken:          "fake-token",
		PluggableTransport: "obfs4-tcp",
		Cert:               "1o+SteGwt6onzK3pEhu1C2XDcKm3x6hgFuH89paQY7noEG7/O9wBtEfwvCPwUXN5MJrMaA",
		PluggableTransportSettings: map[string]string{
			"iat-mode": "0",
		},
	}
	_, err := ChainedDialer("test", s, newTestUserConfig())
	return err
}
