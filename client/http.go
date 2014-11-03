package client

import (
	"net/http"

)

// HttpClient creates a simple domain-fronted HTTP client using the specified
// values for the upstream host to use and for the masquerade/domain fronted host.
func HttpClient(serverInfo *ServerInfo, masquerade *Masquerade) *http.Client {
	if masquerade != nil && masquerade.RootCA == "" {
		serverInfo.InsecureSkipVerify = true
	}

	enproxyConfig := serverInfo.disposableEnproxyConfig(masquerade)

	return &http.Client{
		Transport: &http.Transport{
			Dial: enproxyConfigDialer(enproxyConfig),
		},
	}
}
