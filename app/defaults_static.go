// +build disableresourcerandomization

package app

const (
	defaultHTTPProxyAddress  = "127.0.0.1:8787"
	defaultSOCKSProxyAddress = "127.0.0.1:8788"
)

// localHTTPToken is a no-op without resource randomization.
func localHTTPToken(set *Settings) string {
	return ""
}
