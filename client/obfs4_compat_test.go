package client

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestOBFS4BackwardCompatibility(t *testing.T) {
	assert.NoError(t, buildDialer(), "obfs4 protocol in old config format should have been converted to obfs4-tcp")
}

func buildDialer() error {
	s := &ChainedServerInfo{
		PluggableTransport: "obfs4-tcp",
		Cert:               "1o+SteGwt6onzK3pEhu1C2XDcKm3x6hgFuH89paQY7noEG7/O9wBtEfwvCPwUXN5MJrMaA",
		PluggableTransportSettings: map[string]string{
			"iat-mode": "0",
		},
	}
	_, err := ChainedDialer("test", s, "deviceid", func() string { return "protoken" })
	return err
}
