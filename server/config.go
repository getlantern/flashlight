package server

type ServerConfig struct {
	Portmap        int
	AdvertisedHost string // FQDN that is guaranteed to hit this server
}
