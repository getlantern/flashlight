package client

// ClientConfig captures configuration information for a Client
type ClientConfig struct {
	ShouldDumpHeaders bool // whether or not to dump headers of requests and responses
	Servers           map[string]*ServerInfo
	MasqueradeSets    map[string][]*Masquerade
}

// Masquerade contains the data for a single masquerade host, including
// the domain and the root CA.
type Masquerade struct {
	// Domain: the domain to use for domain fronting
	Domain string

	// RootCA: the root CA for the domain.
	RootCA string
}
