// package cloudflare provides an implementation of a host-spoofing protocol for
// CloudFlare.
package cloudflare

const (
	CF_PREFIX         = "Cf-"
	X_FORWARDED_PROTO = "X-Forwarded-Proto"
	X_FORWARDED_FOR   = "X-Forwarded-For"
	X_LANTERN_PREFIX  = "X-Lantern-"
	X_LANTERN_URL     = X_LANTERN_PREFIX + "URL"
)
