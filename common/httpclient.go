package common

import (
	"net/http"
	"sync"

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
}

func GetHTTPClient() *http.Client {
	mutex.Lock()
	defer mutex.Unlock()
	if httpClient != nil {
		return httpClient
	}
	// Set the client to the kindling client.
	k := kindling.NewKindling(
		kindling.WithDomainFronting("https://media.githubusercontent.com/media/getlantern/fronted/refs/heads/main/fronted.yaml.gz", ""),
		kindling.WithProxyless(domains...),
	)
	httpClient = k.NewHTTPClient()
	return httpClient
}
