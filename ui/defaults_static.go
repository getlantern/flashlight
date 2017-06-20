// +build disableresourcerandomization

package ui

var defaultUIAddresses = []string{"localhost:16823"}

const strictOriginCheck = false

// LocalHTTPToken is a no-op without resource randomization.
func LocalHTTPToken() string {
	// no-op.
	return ""
}
