package config

// ServerLocation represents the location info embeded in proxy config.
type ServerLocation struct {
	City        string
	Country     string
	CountryCode string
	Latitude    float32
	Longitude   float32
}

// ChainedServerInfo contains all the data for connecting to a given chained
// server.
type ChainedServerInfo struct {
	// Addr: the host:port of the upstream proxy server
	Addr string

	// Cert: optional PEM encoded certificate for the server. If specified,
	// server will be dialed using TLS over tcp. Otherwise, server will be
	// dialed using plain tcp. For OBFS4 proxies, this is the Base64-encoded obfs4
	// certificate.
	Cert string

	// AuthToken: the authtoken to present to the upstream server.
	AuthToken string

	// Trusted: Determines if a host can be trusted with plain HTTP traffic.
	Trusted bool

	// InitPreconnect: how much to preconnect on startup
	InitPreconnect int

	// MaxPreconnect: the maximum number of preconnections to keep
	MaxPreconnect int

	// Bias indicates a relative biasing factor for proxy selection purposes.
	// Proxies are bias 0 by default, meaning that they're prioritized by the
	// usual bandwidth and RTT metrics. Proxies with a higher bias are
	// preferred over proxies with a lower bias irrespective of their measured
	// performance.
	Bias int

	// PluggableTransport: If specified, a pluggable transport will be used
	PluggableTransport string

	// PluggableTransportSettings: Settings for pluggable transport
	PluggableTransportSettings map[string]string

	// KCPSettings: If specified, traffic will be tunneled over KCP
	KCPSettings map[string]interface{}

	// The URL at which to access a domain-fronting server farm using the enhttp
	// protocol.
	ENHTTPURL string

	// TLSDesktopOrderedCipherSuiteNames: The ordered list of cipher suites to use
	// for desktop clients using TLS represented as strings.
	TLSDesktopOrderedCipherSuiteNames []string

	// TLSMobileOrderedCipherSuiteNames: The ordered list of cipher suites to use
	// for mobile clients using TLS represented as strings. This may differ from
	// the ordering for desktop because performance of AES ciphers is more of a
	// concern on mobile.
	TLSMobileOrderedCipherSuiteNames []string

	// TLSServerNameIndicator: Specifies the hostname that should be sent by the
	// client as the Server Name Indication header in a TLS request.  If not
	// provided, the client should not send an SNI header.
	TLSServerNameIndicator string

	// TLSClientSessionCacheSize: the size of client session cache to use. Set to
	// 0 to use default size, set to < 0 to disable.
	TLSClientSessionCacheSize int

	// TLSClientHelloID specifies with uTLS client hello to use.
	TLSClientHelloID string

	// Location: the location where the server resides.
	Location ServerLocation

	// MultiplexedAddr: a host:port at which this server is reachable using cmux-
	// based multiplexing.
	MultiplexedAddr string

	// MultiplexedPhysicalConns controls how many physical connections to use
	// under the multiplexing. This defaults to 1.
	MultiplexedPhysicalConns int
}
