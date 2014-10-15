package client

import (
	"net"
	"sort"
	"time"

	"github.com/getlantern/nattywad"
)

// ClientConfig captures configuration information for a Client
type ClientConfig struct {
	DumpHeaders    bool // whether or not to dump headers of requests and responses
	Servers        []*ServerInfo
	MasqueradeSets map[string][]*Masquerade
	Peers          []*nattywad.ServerPeer
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

	// BufferRequests: if true, requests to the proxy will be buffered and sent
	// with identity encoding.  If false, they'll be streamed with chunked
	// encoding.
	BufferRequests bool

	// DialTimeoutMillis: how long to wait on dialing server before timing out
	// (defaults to 5 seconds)
	DialTimeoutMillis int

	// RedialAttempts: number of times to try redialing. The total number of
	// dial attempts will be 1 + RedialAttempts.
	RedialAttempts int

	// Weight: relative weight versus other servers (for round-robin)
	Weight int

	// QOS: relative quality of service offered. Should be >= 0, with higher
	// values indicating higher QOS.
	QOS int
}

type cachedConn struct {
	conn   net.Conn
	dialed time.Time
}

// Masquerade contains the data for a single masquerade host, including
// the domain and the root CA.
type Masquerade struct {
	// Domain: the domain to use for domain fronting
	Domain string

	// RootCA: the root CA for the domain.
	RootCA string
}

// SortHosts sorts the Servers array in place, ordered by host
func (c *ClientConfig) SortServers() {
	sort.Sort(ByHost(c.Servers))
}

// ByHost implements sort.Interface for []*ServerInfo based on the host
type ByHost []*ServerInfo

func (a ByHost) Len() int           { return len(a) }
func (a ByHost) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByHost) Less(i, j int) bool { return a[i].Host < a[j].Host }
