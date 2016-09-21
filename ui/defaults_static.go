// +build disableresourcerandomization

package ui

const defaultUIAddress = "127.0.0.1:16823"

const strictOriginCheck = false

func proxyDomainFor(addr string) string {
	return "ui.lantern.io"
}

func token() string {
	return ""
}
