// +build disableresourcerandomization

package ui

const defaultUIAddress = "127.0.0.1:16823"

const strictOriginCheck = false

func proxyDomainFor(addr string) string {
	return "ui.lantern.io"
}

func Token() string {
	return ""
}

// LocalHTTPToken is a no-op without resource randomization.
func LocalHTTPToken() string {
	// no-op.
}
