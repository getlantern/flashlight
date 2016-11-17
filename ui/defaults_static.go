// +build disableresourcerandomization

package ui

const defaultUIAddress = "127.0.0.1:16823"

const strictOriginCheck = false

// LocalHTTPToken is a no-op without resource randomization.
func LocalHTTPToken() string {
	// no-op.
	return ""
}
