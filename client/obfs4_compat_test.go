package client

import (
	"testing"

	"github.com/getlantern/flashlight/chained"
	"github.com/stretchr/testify/assert"
)

const (
	oldStyleName = "myserver"
	newStyleName = "myserver-obfs4"
)

func TestOBFS4BackwardCompatibility(t *testing.T) {
	assert.NoError(t, buildDialer(oldStyleName), "obfs4 protocol in old config format should have been converted to obfs4-tcp")
}

func TestOBFS4BackwardCompatibilityIgnoreNewConfig(t *testing.T) {
	assert.Error(t, buildDialer(newStyleName), "obfs4 protocol in new config format should have been ignored and not converted to obfs4-tcp")
}

func buildDialer(name string) error {
	s := &chained.ChainedServerInfo{
		Addr:               "1.1.1.1",
		AuthToken:          "fake-token",
		PluggableTransport: "obfs4",
		Cert:               "1o+SteGwt6onzK3pEhu1C2XDcKm3x6hgFuH89paQY7noEG7/O9wBtEfwvCPwUXN5MJrMaA",
		PluggableTransportSettings: map[string]string{
			"iat-mode": "0",
		},
	}
	_, err := ChainedDialer(name, s, "deviceid", func() string { return "protoken" })
	return err
}
