package client

import (
	"github.com/getlantern/fronted"
)

// ClientConfig captures configuration information for a Client
type ClientConfig struct {
	ShowAds bool // whether or not to show ads for a given client

	DumpHeaders    bool // whether or not to dump headers of requests and responses
	ChainedServers map[string]*ChainedServerInfo
	MasqueradeSets map[string][]*fronted.Masquerade
}

// NewConfig creates a new client config with default values.
func NewConfig() *ClientConfig {
	return &ClientConfig{
		ChainedServers: make(map[string]*ChainedServerInfo),
		MasqueradeSets: make(map[string][]*fronted.Masquerade),
	}
}
