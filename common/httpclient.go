package common

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/getlantern/flashlight/v7/sentry"
	"github.com/getlantern/fronted"
	"github.com/getlantern/kindling"
)

var httpClient *http.Client
var mutex = &sync.Mutex{}

// These are the domains we will access via kindling.
var domains = []string{
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

var configDir atomic.Value

func SetConfigDir(dir string) {
	configDir.Store(dir)
}

func GetHTTPClient() *http.Client {
	mutex.Lock()
	defer mutex.Unlock()
	if httpClient != nil {
		return httpClient
	}

	ioWriter := log.AsStdLogger().Writer()
	// Create new fronted instance.
	f, err := newFronted(ioWriter, sentry.PanicListener)
	if err != nil {
		log.Errorf("Failed to create fronted instance: %v", err)
	}
	// Set the client to the kindling client.
	k := kindling.NewKindling(
		kindling.WithPanicListener(sentry.PanicListener),
		kindling.WithLogWriter(ioWriter),
		kindling.WithDomainFronting(f),
		kindling.WithProxyless(domains...),
	)
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
	if configDir.Load() == nil {
		cacheFile = filepath.Join(configDir.Load().(string), "fronted_cache.json")
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
