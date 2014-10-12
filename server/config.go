package server

type ServerConfig struct {
	Portmap        int
	AdvertisedHost string // FQDN that is guaranteed to hit this server
	WaddellAddr    string // Address at which to connect to waddell for signaling
}
