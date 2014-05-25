// package proxy provides the implementations of the client and server proxies
package proxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	"code.google.com/p/go-uuid/uuid"

	"github.com/davecgh/go-spew/spew"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/go-reverseproxy/rp"
	"github.com/getlantern/keyman"
)

// ProxyConfig encapsulates common proxy configuration
type ProxyConfig struct {
	Addr              string        // listen address in form of host:port
	CertContext       *CertContext  // context for certificate management
	ShouldDumpHeaders bool          // whether or not to dump headers of requests and responses
	TLSConfig         *tls.Config   // (optional) TLS configuration for inbound connections, if nil then DEFAULT_TLS_SERVER_CONFIG is used
	ReadTimeout       time.Duration // (optional) timeout for read ops
	WriteTimeout      time.Duration // (optional) timeout for write ops
	reverseProxy      *rp.ReverseProxy
}

// CertContext encapsulates the certificates used by a Proxy
type CertContext struct {
	PKFile         string
	CACertFile     string
	ServerCertFile string
	pk             *keyman.PrivateKey
	caCert         *keyman.Certificate
	serverCert     *keyman.Certificate
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

// InitCommonCerts initializes a private key and CA certificate, used both for
// the server HTTPS proxy and the client MITM proxy.  The key and certificate
// are generated if not already present. The CA  certificate is added to the
// current user's trust store (e.g. keychain) as a trusted root if one with the
// same common name is not already present.
func (ctx *CertContext) InitCommonCerts() (err error) {
	if ctx.pk, err = keyman.LoadPKFromFile(ctx.PKFile); err != nil {
		if os.IsNotExist(err) {
			log.Debugf("Creating new PK at: %s", ctx.PKFile)
			if ctx.pk, err = keyman.GeneratePK(2048); err != nil {
				return
			}
			if err = ctx.pk.WriteToFile(ctx.PKFile); err != nil {
				return fmt.Errorf("Unable to save private key: %s", err)
			}
		} else {
			return fmt.Errorf("Unable to read private key, even though it exists: %s", err)
		}
	}

	ctx.caCert, err = keyman.LoadCertificateFromFile(ctx.CACertFile)
	if err != nil || ctx.caCert.ExpiresBefore(ONE_MONTH_FROM_TODAY) {
		if os.IsNotExist(err) {
			log.Debugf("Creating new self-signed CA cert at: %s", ctx.CACertFile)
			if ctx.caCert, err = ctx.certificateFor(FLASHLIGHT_CN_PREFIX+uuid.New(), TEN_YEARS_FROM_TODAY, true, nil); err != nil {
				return
			}
			if err = ctx.caCert.WriteToFile(ctx.CACertFile); err != nil {
				return fmt.Errorf("Unable to save CA certificate: %s", err)
			}
		} else {
			return fmt.Errorf("Unable to read CA cert, even though it exists: %s", err)
		}
	}

	return nil
}

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

// writhRewrite creates a RoundTripper that uses the supplied RoundTripper and
// rewrites the response.
func withRewrite(rw func(*http.Response), dumpHeaders bool, rt http.RoundTripper) http.RoundTripper {
	return &wrappedRoundTripper{
		rewrite:     rw,
		orig:        rt,
		dumpHeaders: dumpHeaders,
	}
}

// wrappedRoundTripper is an http.RoundTripper that wraps another
// http.RoundTripper to rewrite responses using the rewrite function prior to
// returning them.
type wrappedRoundTripper struct {
	rewrite     func(*http.Response)
	orig        http.RoundTripper
	dumpHeaders bool
}

func (rt *wrappedRoundTripper) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	resp, err = rt.orig.RoundTrip(req)
	if err == nil {
		rt.rewrite(resp)
		if rt.dumpHeaders {
			dumpHeaders("Response", resp.Header)
		}
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
