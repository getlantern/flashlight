// +build disableresourcerandomization

package localurl

const defaultUIAddress = "localhost:16823"

const strictOriginCheck = false

// localHTTPToken is a no-op without resource randomization.
func localHTTPToken() string {
	// no-op.
	return ""
}
