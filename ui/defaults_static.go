// +build disableresourcerandomization

package ui

const defaultUIAddress = "127.0.0.1:16823"

const strictOriginCheck = false

func proxyDomain() string {
	return "http://ui.lantern.io"
}

func token() string {
	return ""
}
