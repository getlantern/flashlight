package domains

import (
	"net"
	"net/http"
	"sort"
	"strings"
)

// Config represents the configuration for a given domain
type Config struct {
	// Unthrottled indicates that this domain should not be subject to throttling.
	Unthrottled bool

	// RewriteToHTTPS indicates that HTTP requests to this domain should be
	// rewritten to HTTPS.
	RewriteToHTTPS bool

	// AddConfigServerHeaders indicates that we should add config server auth
	// tokens and client IP headers on requests to this domain
	AddConfigServerHeaders bool

	// AddForwardedFor indicates that we should include an X-Forwarded-For header
	// with the client's IP.
	AddForwardedFor bool

	// PassInternalHeaders indicates that headers starting with X-Lantern-* should
	// be passed to this domain.
	PassInternalHeaders bool
}

func (cfg *Config) withRewriteToHTTPS() *Config {
	var cfg2 = *cfg
	cfg2.RewriteToHTTPS = true
	return &cfg2
}

func (cfg *Config) withAddConfigServerHeaders() *Config {
	var cfg2 = *cfg
	cfg2.AddConfigServerHeaders = true
	return &cfg2
}

// ConfigWithHost is a Config with associated hostname/domain
type ConfigWithHost struct {
	Host string
	Config
}

var (
	internal = &Config{
		Unthrottled:         true,
		AddForwardedFor:     true,
		PassInternalHeaders: true,
	}

	externalUnthrottled = &Config{
		Unthrottled: true,
	}
)

var configs = configure(
	map[string]*Config{
		"config.getiantem.org":         internal.withRewriteToHTTPS().withAddConfigServerHeaders(),
		"config-staging.getiantem.org": internal.withRewriteToHTTPS().withAddConfigServerHeaders(),

		// These are the config server domains Beam uses.
		"config.ss7hc6jm.io":                       internal.withRewriteToHTTPS().withAddConfigServerHeaders(),
		"config-staging.ss7hc6jm.io":               internal.withRewriteToHTTPS().withAddConfigServerHeaders(),
		"api.getiantem.org":                        internal.withRewriteToHTTPS(),
		"api-staging.getiantem.org":                internal.withRewriteToHTTPS(),
		"replica-search.lantern.io":                internal.withRewriteToHTTPS(),
		"replica-search-aws.lantern.io":            internal.withRewriteToHTTPS(),
		"replica-search-ir.lantern.io":             internal.withRewriteToHTTPS(),
		"replica-frankfurt.lantern.io":             internal.withRewriteToHTTPS(),
		"replica-search-staging.lantern.io":        internal.withRewriteToHTTPS(),
		"replica-thumbnailer.lantern.io":           internal.withRewriteToHTTPS(),
		"replica-thumbnailer-staging.lantern.io":   internal.withRewriteToHTTPS(),
		"getlantern.org":                           internal,
		"lantern.io":                               internal,
		"innovatelabs.io":                          internal,
		"getiantem.org":                            internal,
		"lantern-pro-server.herokuapp.com":         internal,
		"lantern-pro-server-staging.herokuapp.com": internal,
		"adyenpayments.com":                        externalUnthrottled,
		"adyen.com":                                externalUnthrottled,
		"stripe.com":                               externalUnthrottled,
		"paymentwall.com":                          externalUnthrottled,
		"alipay.com":                               externalUnthrottled,
		"app-measurement.com":                      externalUnthrottled,
		"fastworldpay.com":                         externalUnthrottled,
		"firebaseremoteconfig.googleapis.com":      externalUnthrottled,
		"firebaseio.com":                           externalUnthrottled,
		"optimizely.com":                           externalUnthrottled,
	})

// ConfigForRequest is like ConfigForHost, using the hostname part of req.Host
// from the given request.
func ConfigForRequest(req *http.Request) *ConfigWithHost {
	return ConfigForAddress(req.Host)
}

// ConfigForAddress returns the config for the deepest matching sub-domain for
// the host portion of the given network address.
func ConfigForAddress(addr string) *ConfigWithHost {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return ConfigForHost(host)
}

// ConfigForHost returns the config for the deepest matching sub-domain for the
// given host.
func ConfigForHost(host string) *ConfigWithHost {
	host = strings.ToLower(host)
	cfg := &ConfigWithHost{Host: host}

	for _, dcfg := range configs {
		if host == dcfg.Host || strings.HasSuffix(host, "."+dcfg.Host) {
			cfg.Config = dcfg.Config
			return cfg
		}
	}

	return cfg
}

func configure(m map[string]*Config) []*ConfigWithHost {
	cfgs := make([]*ConfigWithHost, 0, len(m))
	for domain, config := range m {
		cfgs = append(cfgs, &ConfigWithHost{Host: domain, Config: *config})
	}
	sort.Sort(byDepth(cfgs))
	return cfgs
}

type byDepth []*ConfigWithHost

func (a byDepth) Len() int      { return len(a) }
func (a byDepth) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byDepth) Less(i, j int) bool {
	iDomain, jDomain := a[i].Host, a[j].Host
	iParts := strings.Split(iDomain, ".")
	jParts := strings.Split(jDomain, ".")
	if len(iParts) > len(jParts) {
		return true
	}
	if len(iParts) < len(jParts) {
		return false
	}
	// Equal length, sort in reverse order by domain parts
	for x := len(iParts) - 1; x >= 0; x-- {
		ip, jp := iParts[x], jParts[x]
		if ip < jp {
			return true
		}
		if ip > jp {
			return false
		}
	}
	return false
}
