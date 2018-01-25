package client

import (
	"github.com/getlantern/fronted"
)

// ClientConfig captures configuration information for a Client
type ClientConfig struct {
	DumpHeaders    bool // whether or not to dump headers of requests and responses
	MasqueradeSets map[string][]*fronted.Masquerade
}

// NewConfig creates a new client config with default values.
func NewConfig() *ClientConfig {
	return &ClientConfig{
		MasqueradeSets: make(map[string][]*fronted.Masquerade),
	}
}
