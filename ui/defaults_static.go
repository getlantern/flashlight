// +build disableresourcerandomization

package ui

const defaultUIAddress = "127.0.0.1:16823"

// pacPath returns the legacy proxy on pac address.
func pacPath() string {
	return "/proxy_on.pac"
}

func proxyDomain() string {
	return "ui.lantern.io"
}
