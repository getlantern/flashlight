package common

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/getlantern/amp"
	"github.com/getlantern/dnstt"
	"github.com/getlantern/fronted"
	"github.com/getlantern/kindling"

	"github.com/getlantern/flashlight/v7/sentry"
)

var (
	httpClient *http.Client
	mutex      = &sync.Mutex{}

	// These are the domains we will access via kindling.
	domains = []string{
		"api.iantem.io",
		"api.getiantem.org",    // Still used on iOS
		"geo.getiantem.org",    // Still used on iOS
		"config.getiantem.org", // Still used on iOS
		"df.iantem.io",
		"raw.githubusercontent.com",
		"media.githubusercontent.com",
		"objects.githubusercontent.com",
		"replica-r2.lantern.io",
		"replica-search.lantern.io",
		"update.getlantern.org",
		"globalconfig.flashlightproxy.com",
		"dogsdogs.xyz",         // Used in replica
		"service.dogsdogs.xyz", // Used in replica
	}

	configDir      atomic.Value
	dnsttConfig    atomic.Value // Holds the DNSTTConfig
	ampCacheConfig atomic.Value

	defaultDNSTTConfig = &DNSTTConfig{
		Domain:           "t.iantem.io",
		PublicKey:        "405eb9e22d806e3a0a8e667c6665a321c8a6a35fa680ed814716a66d7ad84977",
		DoHResolver:      "https://cloudflare-dns.com/dns-query",
		DoTResolver:      "",
		UTLSDistribution: "",
	}
)

type DNSTTConfig struct {
	Domain           string `yaml:"domain"`    // DNS tunnel domain, e.g., "t.iantem.io"
	PublicKey        string `yaml:"publicKey"` // DNSTT server public key
	DoHResolver      string `yaml:"dohResolver,omitempty"`
	DoTResolver      string `yaml:"dotResolver,omitempty"`
	UTLSDistribution string `yaml:"utlsDistribution,omitempty"`
}

// AMPCacheConfig contain the properties required for starting the
// amp cache fronting
type AMPCacheConfig struct {
	BrokerURL    string   `yaml:"broker_url"`
	CacheURL     string   `yaml:"cache_url"`
	PublicKeyPEM string   `yaml:"public_key"`
	FrontDomains []string `yaml:"front_domains"`
}

func init() {
	dnsttConfig.Store(defaultDNSTTConfig)
}

// SetConfigDir set the config directory on some platforms, such as Android, can only be determined in native code, so we
// need to set it externally.
func SetConfigDir(dir string) {
	configDir.Store(dir)
}

func SetDNSTTConfig(cfg *DNSTTConfig) {
	if cfg != nil {
		dnsttConfig.Store(cfg)
	}
}

func SetAMPCacheConfig(cfg *AMPCacheConfig) {
	if cfg != nil {
		ampCacheConfig.Store(cfg)
	}
}

func GetHTTPClient() *http.Client {
	mutex.Lock()
	defer mutex.Unlock()
	if httpClient != nil {
		return httpClient
	}

	return NewHTTPClient()
}

// NewHTTPClient build a new http client and store it on the httpClient
// package var. This function should be called when there's a configuration
// update.
func NewHTTPClient() *http.Client {
	mutex.Lock()
	defer mutex.Unlock()
	var k kindling.Kindling
	ioWriter := log.AsDebugLogger().Writer()
	kOptions := []kindling.Option{
		kindling.WithPanicListener(sentry.PanicListener),
		kindling.WithLogWriter(ioWriter),
		kindling.WithProxyless(domains...),
	}

	// Create new fronted instance.
	f, err := newFronted(ioWriter, sentry.PanicListener)
	if err != nil {
		log.Errorf("Failed to create fronted instance: %v", err)
	} else {
		kOptions = append(kOptions, kindling.WithDomainFronting(f))
	}
	if d, err := newDNSTT(); err != nil {
		log.Errorf("Failed to create DNSTT: %v", err)
	} else {
		kOptions = append(kOptions, kindling.WithDNSTunnel(d))
	}

	if ampClient, err := newAMPCache(); err != nil {
		log.Errorf("Failed to create AMP cache client: %v", err)
	} else {
		kOptions = append(kOptions, kindling.WithAMPCache(ampClient))
	}
	k = kindling.NewKindling("flashlight", kOptions...)
	httpClient = k.NewHTTPClient()
	return httpClient
}

func newFronted(logWriter io.Writer, panicListener func(string)) (fronted.Fronted, error) {
	// Parse the domain from the URL.
	configURL := "https://raw.githubusercontent.com/getlantern/lantern-binaries/refs/heads/main/fronted.yaml.gz"
	u, err := url.Parse(configURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %v", err)
	}
	// Extract the domain from the URL.
	domain := u.Host

	// First, download the file from the specified URL using the smart dialer.
	// Then, create a new fronted instance with the downloaded file.
	trans, err := kindling.NewSmartHTTPTransport(logWriter, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to create smart HTTP transport: %v", err)
	}
	httpClient := &http.Client{
		Transport: trans,
	}
	var cacheFile string
	configDirValue := configDir.Load()
	if configDirValue != nil {
		cacheFile = filepath.Join(configDirValue.(string), "fronted_cache.json")
	} else {
		cacheFile = filepath.Join(os.TempDir(), "fronted_cache.json")
	}
	return fronted.NewFronted(
		fronted.WithPanicListener(panicListener),
		fronted.WithCacheFile(cacheFile),
		fronted.WithHTTPClient(httpClient),
		fronted.WithConfigURL(configURL),
	), nil
}

func (c *DNSTTConfig) Validate() error {
	if c.PublicKey == "" {
		return fmt.Errorf("publicKey is required")
	}
	if c.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	if c.DoHResolver == "" && c.DoTResolver == "" {
		return fmt.Errorf("at least one of DoHResolver or DoTResolver must be specified")
	}
	return nil
}

func (c *AMPCacheConfig) Validate() error {
	if c.BrokerURL == "" ||
		c.CacheURL == "" ||
		len(c.FrontDomains) == 0 ||
		c.PublicKeyPEM == "" {
		return fmt.Errorf(
			`missing one or more mandatory arguments for running amp cache fronting.
			Please review the provided parameter values, broker URL: %q, cache URL: %q, front domains: %q, public key: %q`,
			c.BrokerURL, c.CacheURL, c.FrontDomains, c.PublicKeyPEM,
		)
	}
	return nil
}

func newDNSTT() (dnstt.DNSTT, error) {
	cfg := dnsttConfig.Load().(*DNSTTConfig)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid DNSTT configuration: %w", err)
	}

	options := []dnstt.Option{
		dnstt.WithPublicKey(cfg.PublicKey),
		dnstt.WithTunnelDomain(cfg.Domain),
	}
	switch {
	case cfg.DoHResolver != "":
		options = append(options, dnstt.WithDoH(cfg.DoHResolver))
	case cfg.DoTResolver != "":
		options = append(options, dnstt.WithDoT(cfg.DoTResolver))
	}
	if cfg.UTLSDistribution != "" {
		options = append(options, dnstt.WithUTLSDistribution(cfg.UTLSDistribution))
	}
	return dnstt.NewDNSTT(options...)
}

func newAMPCache() (amp.Client, error) {
	cfg := ampCacheConfig.Load()
	if cfg == nil {
		return nil, fmt.Errorf("amp cache config not available")
	}

	config, ok := cfg.(*AMPCacheConfig)
	if !ok {
		return nil, fmt.Errorf("invalid data stored in amp cache config")
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid AMP cache configuration: %w", err)
	}

	brokerURL, err := url.Parse(config.BrokerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid broker url: %w", err)
	}

	cacheURL, err := url.Parse(config.CacheURL)
	if err != nil {
		return nil, fmt.Errorf("invalid cache url: %w", err)
	}

	// parse rsa public key pem
	pubKey, err := parsePublicKeyPEM(config.PublicKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("invalid public key pem: %w", err)
	}

	return amp.NewClient(brokerURL, cacheURL, config.FrontDomains, nil, pubKey, func(network, address string) (net.Conn, error) {
		return tls.Dial("tcp", fmt.Sprintf("%s:443", address), &tls.Config{
			ServerName: address,
		})
	})
}

func parsePublicKeyPEM(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DER encoded public key: %v", err)
	}
	key, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not RSA public key")
	}
	return key, nil
}
