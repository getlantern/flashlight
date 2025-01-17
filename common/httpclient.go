package common

import (
	"net/http"
	"sync"

	"github.com/getlantern/kindling"
)

var httpClient *http.Client
var mutex = &sync.Mutex{}

func GetHTTPClient() *http.Client {
	mutex.Lock()
	defer mutex.Unlock()
	if httpClient != nil {
		return httpClient
	}
	// Set the client to the kindling client.
	k := kindling.NewKindling(
		kindling.WithDomainFronting("https://media.githubusercontent.com/media/getlantern/fronted/refs/heads/main/fronted.yaml.gz", ""),
		kindling.WithProxyless("api.iantem.io", "iantem.io", "df.iantem.io"),
	)
	httpClient = k.NewHTTPClient()
	return httpClient
}
