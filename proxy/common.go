// package proxy provides the implementations of the client and server proxies
package proxy

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/keyman"
)

// ProxyConfig encapsulates common proxy configuration
type ProxyConfig struct {
	Addr              string        // listen address in form of host:port
	ShouldDumpHeaders bool          // whether or not to dump headers of requests and responses
	ReadTimeout       time.Duration // (optional) timeout for read ops
	WriteTimeout      time.Duration // (optional) timeout for write ops
	TLSConfig         *tls.Config   // (optional) TLS configuration for inbound connections, if nil then DEFAULT_TLS_SERVER_CONFIG is used
}

const (
	CONNECT                      = "CONNECT"                // HTTP CONNECT method
	X_LANTERN_REQUEST_INFO       = "X-Lantern-Request-Info" // Tells proxy to return info about the client
	X_LANTERN_PUBLIC_IP          = "X-LANTERN-PUBLIC-IP"    // Client's public IP as seen by the proxy
	TLS_SESSIONS_TO_CACHE_SERVER = 100000

	FLASHLIGHT_CN_PREFIX = "flashlight-" // prefix for common-name on generated certificates

	HR = "--------------------------------------------------------------------------------"
)

var (
	// Points in time, mostly used for generating certificates
	TOMORROW             = time.Now().AddDate(0, 0, 1)
	ONE_MONTH_FROM_TODAY = time.Now().AddDate(0, 1, 0)
	ONE_YEAR_FROM_TODAY  = time.Now().AddDate(1, 0, 0)
	TEN_YEARS_FROM_TODAY = time.Now().AddDate(10, 0, 0)

	STREAMING_FLUSH_INTERVAL = 50 * time.Millisecond

	// Default TLS configuration for servers
	DEFAULT_TLS_SERVER_CONFIG = &tls.Config{
		// The ECDHE cipher suites are preferred for performance and forward
		// secrecy.  See https://community.qualys.com/blogs/securitylabs/2013/06/25/ssl-labs-deploying-forward-secrecy.
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
			tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
			tls.TLS_RSA_WITH_RC4_128_SHA,
			tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
	}
)

// certificateFor generates a certificate for a given name, signed by the given
// issuer.  If no issuer is specified, the generated certificate is
// self-signed.
func (ctx *CertContext) certificateFor(
	name string,
	validUntil time.Time,
	isCA bool,
	issuer *keyman.Certificate) (cert *keyman.Certificate, err error) {
	return ctx.pk.TLSCertificateFor("Lantern", name, validUntil, isCA, issuer)
}

// withDumpHeaders creates a RoundTripper that uses the supplied RoundTripper
// and that dumps headers (if dumpHeaders is true).
func withDumpHeaders(dumpHeaders bool, rt http.RoundTripper) http.RoundTripper {
	if !dumpHeaders {
		return rt
	}
	return &headerDumpingRoundTripper{rt}
}

// headerDumpingRoundTripper is an http.RoundTripper that wraps another
// http.RoundTripper and dumps response headers to the log.
type headerDumpingRoundTripper struct {
	orig http.RoundTripper
}

func (rt *headerDumpingRoundTripper) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	dumpHeaders("Request", req.Header)
	resp, err = rt.orig.RoundTrip(req)
	if err == nil {
		dumpHeaders("Response", resp.Header)
	}
	return
}

// dumpHeaders logs the given headers (request or response).
func dumpHeaders(category string, headers http.Header) {
	log.Debugf("%s Headers\n%s\n%s\n%s\n\n", category, HR, spew.Sdump(headers), HR)
}

// flushIntervalFor determines the flush interval for a given request/response
// pair
func flushIntervalFor(req *http.Request, res *http.Response) time.Duration {
	if res.Header.Get("Content-type") == "text/event-stream" {
		return STREAMING_FLUSH_INTERVAL
	}
	return 0
}
