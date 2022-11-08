// Package mitm provides a facility for man-in-the-middling pairs of
// connections.
package mitm

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/go-cache/cache"
	"github.com/getlantern/keyman"
	"github.com/getlantern/netx"
	"github.com/getlantern/reconn"
)

const (
	oneDay   = 24 * time.Hour
	twoWeeks = oneDay * 14
	oneMonth = 1
	oneYear  = 1
	tenYears = 10 * oneYear

	maxTLSRecordSize = 2 << 15
)

// Opts provides options to configure mitm
type Opts struct {
	// PKFile: the PEM-encoded file to use as the primary key for this server
	PKFile string

	// CertFile: the PEM-encoded X509 certificate to use for this server (must match PKFile)
	CertFile string

	// Organization: Name of the organization to use on the generated CA cert for this  (defaults to "Lantern")
	Organization string

	// Domains: list of domain names to use as Subject Alternate Names
	Domains []string

	// InstallCert: If true, the cert will be installed to the system's keystore
	InstallCert bool

	// InstallPrompt: the text to use when prompting the user to install the MITM
	// cert to the system's keystore.
	InstallPrompt string

	// WindowsPromptTitle: on windows, the certificate installation will actually
	// use a system-standard escalation prompt, but we can first prompt with a
	// dialog box using this title in order to prepare the user for what's coming.
	WindowsPromptTitle string

	// WindowsPromptBody: dialog body to go with WindowsPromptTitle
	WindowsPromptBody string

	// InstallCertResult: optional callback that gets invoked whenever the user is prompted to install a cert.
	// If err is nil, the cert was installed successfully
	InstallCertResult func(err error)

	// ServerTLSConfig: optional configuration for TLS server when MITMing (if nil, a sensible default is used)
	ServerTLSConfig *tls.Config

	// ClientTLSConfig: optional configuration for TLS client when MITMing (if nil, a sensible default is used)
	ClientTLSConfig *tls.Config
}

// Configure creates an MITM that can man-in-the-middle a pair of connections.
// The hostname is determined using SNI. If no SNI header is present, then the
// connection is not MITM'ed. The primary key and certificate used to generate
// and sign MITM certificates are auto-created if not already present.
func Configure(opts *Opts) (*Interceptor, error) {
	if opts.InstallPrompt == "" {
		opts.InstallPrompt = "Please enter your password to install certificate"
	}
	ic := &Interceptor{
		opts:         opts,
		dynamicCerts: cache.NewCache(),
	}
	err := ic.initCrypto()
	if err != nil {
		return nil, err
	}
	return ic, nil
}

// Interceptor provides a facility for MITM'ing pairs of connections.
type Interceptor struct {
	opts            *Opts
	pk              *keyman.PrivateKey
	pkPem           []byte
	issuingCertFile string
	issuingCert     *keyman.Certificate
	issuingCertPem  []byte
	serverTLSConfig *tls.Config
	clientTLSConfig *tls.Config
	dynamicCerts    *cache.Cache
	certMutex       sync.Mutex
}

// MITM man-in-the-middles a pair of connections, returning the connections that
// should be used in place of the originals. If the original connections can't
// be MITM'ed but can continue to be used as-is, those will be returned.
func (ic *Interceptor) MITM(downstream net.Conn, upstream net.Conn) (newDown net.Conn, newUp net.Conn, success bool, err error) {
	rc := reconn.Wrap(downstream, maxTLSRecordSize)
	adDown := &alertDetectingConn{Conn: rc}
	tlsDown := tls.Server(adDown, ic.serverTLSConfig)
	handshakeErr := tlsDown.Handshake()
	if handshakeErr == nil {
		skipTLS := false
		netx.WalkWrapped(upstream, func(wrapped net.Conn) bool {
			_, dontEncrypt := wrapped.(dontEncryptConn)
			if dontEncrypt {
				skipTLS = true
			}
			return !dontEncrypt
		})
		if skipTLS {
			return tlsDown, upstream, true, nil
		}
		tlsConfig := makeConfig(ic.clientTLSConfig)
		tlsConfig.ServerName = tlsDown.ConnectionState().ServerName
		tlsUp := tls.Client(upstream, tlsConfig)
		return tlsDown, tlsUp, true, tlsUp.Handshake()
	} else if adDown.sawAlert() || errors.As(handshakeErr, &tls.RecordHeaderError{}) {
		// Don't MITM, send any received handshake info on to upstream
		rr, err := rc.Rereader()
		if err != nil {
			return nil, nil, false, fmt.Errorf("Unable to re-attempt TLS connection to upstream: %v", err)
		}
		_, err = io.Copy(upstream, rr)
		if err != nil {
			return nil, nil, false, err
		}
		return rc, upstream, false, nil
	}
	return nil, nil, false, handshakeErr
}

func (ic *Interceptor) initCrypto() (err error) {
	if len(ic.opts.Domains) == 0 {
		return fmt.Errorf("MITM options need at least one domain")
	}
	if ic.opts.Organization == "" {
		ic.opts.Organization = "Lantern"
	}

	// Load/generate the private key
	if ic.pk, err = keyman.LoadPKFromFile(ic.opts.PKFile); err != nil {
		ic.pk, err = keyman.GeneratePK(2048)
		if err != nil {
			return fmt.Errorf("Unable to generate private key: %s", err)
		}
		ic.pk.WriteToFile(ic.opts.PKFile)
	}
	ic.pkPem = ic.pk.PEMEncoded()

	publicKey := ic.pk.RSA().PublicKey
	// Use a hash of the modulues and exponent of the public key plus and all domains as the common name for our cert (unique key)
	certificateKey := fmt.Sprintf("%v_%v_%v", publicKey.N, publicKey.E, strings.Join(ic.opts.Domains, ","))
	certificateHash := sha256.Sum256([]byte(certificateKey))
	certID := hex.EncodeToString(certificateHash[:])
	ic.issuingCertFile = fmt.Sprintf("%v_%v", ic.opts.CertFile, certID)

	ic.issuingCert, err = keyman.LoadCertificateFromFile(ic.issuingCertFile)

	if err != nil || ic.issuingCert.ExpiresBefore(time.Now().AddDate(0, oneMonth, 0)) {
		ic.issuingCert, err = ic.pk.TLSCertificateFor(
			time.Now().AddDate(tenYears, 0, 0),
			false,
			nil,
			ic.opts.Organization,
			fmt.Sprintf("%v_%v", ic.opts.Organization, certID),
			ic.opts.Domains...,
		)
		if err != nil {
			return fmt.Errorf("Unable to generate self-signed issuing certificate: %s", err)
		}
		ic.issuingCert.WriteToFile(ic.issuingCertFile)
	}
	ic.issuingCertPem = ic.issuingCert.PEMEncoded()
	if ic.opts.InstallCert {

		if err = ic.issuingCert.AddAsTrustedRootIfNeeded(ic.opts.InstallPrompt, ic.opts.WindowsPromptTitle, ic.opts.WindowsPromptBody, func(e error) {
			if ic.opts.InstallCertResult != nil {
				ic.opts.InstallCertResult(e)
			}
		}); err != nil {
			return fmt.Errorf("unable to install issuing cert: %v", err)
		}
	}

	ic.serverTLSConfig = makeConfig(ic.opts.ServerTLSConfig)
	ic.serverTLSConfig.GetCertificate = ic.makeCertificate

	ic.clientTLSConfig = ic.opts.ClientTLSConfig
	return
}

func (ic *Interceptor) makeCertificate(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := clientHello.ServerName
	if name == "" {
		return nil, fmt.Errorf("No ServerName provided")
	}

	keyPair, err := tls.X509KeyPair(ic.issuingCertPem, ic.pkPem)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse keypair for tls: %s", err)
	}

	// Add to cache, set to expire 1 day before the cert expires
	cacheTTL := 365*24*time.Hour - oneDay
	ic.dynamicCerts.Set(name, &keyPair, cacheTTL)
	return &keyPair, nil
}

// makeConfig makes a copy of a tls config if provided. Otherwise returns an
// empty tls config.
func makeConfig(template *tls.Config) *tls.Config {
	tlsConfig := &tls.Config{}
	if template != nil {
		// Copy the provided tlsConfig
		*tlsConfig = *template
	}
	return tlsConfig
}

// dontEncryptConn is a marker interface for upstream connections that shouldn't
// be encrypted.
type dontEncryptConn interface {
	net.Conn

	// Implement this method to tell mitm to skip encrypting on this connection
	MITMSkipEncryption()
}
