package client

import (
	"log"
	"strconv"
)

// ClientConfig captures configuration information for a Client
type ClientConfig struct {
	DumpHeaders    string // whether or not to dump headers of requests and responses
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

func (cfg *ClientConfig) ShouldDumpHeaders() bool {
	b, _ := strconv.ParseBool(cfg.DumpHeaders)
	return b
}

// UpdateFrom updates this ClientConfig with the values in the provided
// ClientConfig.
func (cfg *ClientConfig) UpdateFrom(newer *ClientConfig) {
	log.Printf("Updating client config")
	if newer.DumpHeaders != "" {
		cfg.DumpHeaders = newer.DumpHeaders
	}
	if newer.MasqueradeSets != nil {
		cfg.updateMasqueradeSetsFrom(newer.MasqueradeSets)
	}
	if newer.Servers != nil {
		cfg.updateServersFrom(newer.Servers)
	}
}

func (cfg *ClientConfig) updateMasqueradeSetsFrom(newer map[string][]*Masquerade) {
	log.Printf("Updating masquerade sets")
	if cfg.MasqueradeSets == nil {
		cfg.MasqueradeSets = make(map[string][]*Masquerade)
	}
	for name, set := range newer {
		log.Printf("Replacing masquerade set: %s", name)
		// Replace named masquerade sets wholesale, which allows us to
		// add and remove masquerade hosts from the set
		cfg.MasqueradeSets[name] = set
	}
}

func (cfg *ClientConfig) updateServersFrom(newer []*ServerInfo) {
	// Organize servers into maps by host name
	servers := make(map[string]*ServerInfo)
	newerServers := make(map[string]*ServerInfo)

	if cfg.Servers != nil {
		for _, server := range cfg.Servers {
			servers[server.Host] = server
		}
	}

	for _, server := range newer {
		newerServers[server.Host] = server
	}

	// Merge newer servers into existing
	for name, newerServer := range newerServers {
		existing := servers[name]
		if existing == nil {
			servers[name] = newerServer
		} else {
			existing.updateFrom(newerServer)
		}
	}

	// Transform merged map of servers into array
	cfg.Servers = make([]*ServerInfo, len(servers))
	i := 0
	for _, server := range servers {
		cfg.Servers[i] = server
		i = i + 1
	}
}

func (serverInfo *ServerInfo) updateFrom(newer *ServerInfo) {
	if newer.Host != "" {
		serverInfo.Host = newer.Host
	}
	if newer.Port != 0 {
		serverInfo.Port = newer.Port
	}
	if newer.MasqueradeSet != "" {
		serverInfo.MasqueradeSet = newer.MasqueradeSet
	}
	if newer.InsecureSkipVerify {
		serverInfo.InsecureSkipVerify = newer.InsecureSkipVerify
	}
	if newer.DialTimeoutMillis != 0 {
		serverInfo.DialTimeoutMillis = newer.DialTimeoutMillis
	}
	if newer.KeepAliveMillis != 0 {
		serverInfo.KeepAliveMillis = newer.KeepAliveMillis
	}
	if newer.Weight != 0 {
		serverInfo.Weight = newer.Weight
	}
	if newer.QOS != 0 {
		serverInfo.QOS = newer.QOS
	}
}
