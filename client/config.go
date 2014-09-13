package client

// ClientConfig captures configuration information for a Client
type ClientConfig struct {
	DumpHeaders    bool // whether or not to dump headers of requests and responses
	Servers        []*ServerInfo
	MasqueradeSets map[string][]*Masquerade
}

// ServerInfo captures configuration information for an upstream server
type ServerInfo struct {
	// Host: the host (e.g. getiantem.org)
	Host string

	// Port: the port (e.g. 443)
	Port int

	// MasqueradeSet: the name of the masquerade set from ClientConfig that
	// contains masquerade hosts to use for this server.
	MasqueradeSet string

	// InsecureSkipVerify: if true, server's certificate is not verified.
	InsecureSkipVerify bool

	// DialTimeoutMillis: how long to wait on dialing server before timing out
	// (defaults to 5 seconds)
	DialTimeoutMillis int

	// KeepAliveMillis: interval for TCP keepalives (defaults to 70 seconds)
	KeepAliveMillis int

	// Weight: relative weight versus other servers (for round-robin)
	Weight int

	// QOS: relative quality of service offered.  Should be >= 0, with higher
	// values indicating higher QOS.
	QOS int
}

// Masquerade contains the data for a single masquerade host, including
// the domain and the root CA.
type Masquerade struct {
	// Domain: the domain to use for domain fronting
	Domain string

	// RootCA: the root CA for the domain.
	RootCA string
}
