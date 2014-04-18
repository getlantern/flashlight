// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/getlantern/go-mitm/mitm"
	"github.com/oxtoacart/keyman"
)

const (
	CONNECT          = "CONNECT"
	X_LANTERN_HOST   = "X-Lantern-Host"
	X_LANTERN_SCHEME = "X-Lantern-Scheme"

	PK_FILE          = "proxypk.pem"
	CA_CERT_FILE     = "cacert.pem"
	SERVER_CERT_FILE = "servercert.pem"
)

var (
	TOMORROW             = time.Now().AddDate(0, 0, 1)
	ONE_MONTH_FROM_TODAY = time.Now().AddDate(0, 1, 0)
	ONE_YEAR_FROM_TODAY  = time.Now().AddDate(1, 0, 0)
	TEN_YEARS_FROM_TODAY = time.Now().AddDate(10, 0, 0)

	help         = flag.Bool("help", false, "Get usage help")
	addr         = flag.String("addr", "", "ip:port on which to listen for requests.  When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https")
	upstreamHost = flag.String("server", "", "hostname at which to connect to a server flashlight (always using https).  When specified, this flashlight will run as a client proxy, otherwise it runs as a server")
	upstreamPort = flag.Int("serverPort", 443, "the port on which to connect to the server")
	masqueradeAs = flag.String("masquerade", "", "masquerade host: if specified, flashlight will actually make a request to this host's IP but with a host header corresponding to the 'server' parameter")

	isDownstream, isUpstream bool

	pk                 *keyman.PrivateKey
	caCert, serverCert *keyman.Certificate

	wg sync.WaitGroup

	mitmProxy *mitm.Proxy
)

func init() {
	flag.Parse()
	if *help || *addr == "" {
		flag.Usage()
		os.Exit(1)
	}

	isDownstream = *upstreamHost != ""
	isUpstream = !isDownstream
}

func main() {
	if err := initCerts(strings.Split(*addr, ":")[0]); err != nil {
		log.Fatalf("Unable to initialize certs: %s", err)
	}
	if isDownstream {
		runClient()
		buildMitmProxy()
	} else {
		runServer()
	}
	wg.Wait()
}

// runClient runs the client HTTP proxy server
func runClient() {
	// On the client, use a bunch of CPUs if necessary
	runtime.GOMAXPROCS(4)
	wg.Add(1)

	server := &http.Server{
		Addr:         *addr,
		Handler:      http.HandlerFunc(handleClient),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("About to start client (http) proxy at %s", *addr)
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Unable to start client proxy: %s", err)
		}
		wg.Done()
	}()
}

// buildMitmProxy builds the MITM proxy that the client uses for proxying HTTPS
// requests we have to MITM these because we can't CONNECT tunnel through
// CloudFlare
func buildMitmProxy() {
	var err error
	mitmProxy, err = mitm.NewProxy(PK_FILE, CA_CERT_FILE)
	if err != nil {
		log.Fatalf("Unable to initialize mitm proxy: %s", err)
	}
}

// runServer runs the server HTTPS proxy
func runServer() {
	wg.Add(1)

	server := &http.Server{
		Addr:         *addr,
		Handler:      http.HandlerFunc(handleServer),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("About to start server (https) proxy at %s", *addr)
		if err := server.ListenAndServeTLS(SERVER_CERT_FILE, PK_FILE); err != nil {
			// if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Unable to start server proxy: %s", err)
		}
		wg.Done()
	}()
}

// handleClient handles requests from a local client (e.g. the browser)
func handleClient(resp http.ResponseWriter, req *http.Request) {
	if req.Method == "CONNECT" {
		mitmProxy.Intercept(resp, req)
	} else {
		req.URL.Scheme = "http"
		doHandleClient(resp, req)
	}
}

// doHandleClient does the work of handling client HTTP requests and injecting
// special Lantern headers to work correctly with the upstream server proxy.
func doHandleClient(resp http.ResponseWriter, req *http.Request) {
	log.Println(spew.Sdump(req))

	host := *upstreamHost
	if *masqueradeAs != "" {
		host = *masqueradeAs
	}
	upstreamAddr := fmt.Sprintf("%s:%d", host, *upstreamPort)

	rp := httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Remember the host and scheme that was actually requested
			req.Header.Set(X_LANTERN_HOST, req.Host)
			req.Header.Set(X_LANTERN_SCHEME, req.URL.Scheme)
			req.URL.Scheme = "http"
			// Set our upstream proxy as the host for this request
			req.Host = *upstreamHost
			req.URL.Host = *upstreamHost
		},
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				log.Printf("Using %s to handle request for: %s", upstreamAddr, req.URL.String())
				tlsConfig := &tls.Config{
					ServerName: host,
				}
				return tls.Dial(network, upstreamAddr, tlsConfig)
				// return net.Dial(network, upstreamAddr)
			},
		},
	}
	rp.ServeHTTP(resp, req)
}

// handleClientMITM handles requests to the client-side MITM proxy, making some
// small modifications and then delegating to doHandleClient.
func handleClientMITM(resp http.ResponseWriter, req *http.Request) {
	req.URL.Scheme = "https"
	req.Host = hostIncludingPort(req)
	doHandleClient(resp, req)
}

func hostIncludingPort(req *http.Request) (host string) {
	host = req.Host
	if !strings.Contains(host, ":") {
		host = host + ":443"
	}
	return
}

// handleServer handles requests from a downstream flashlight client
func handleServer(resp http.ResponseWriter, req *http.Request) {
	log.Println(spew.Sdump(req))

	rp := httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// TODO - need to add support for tunneling HTTPS traffic using CONNECT
			req.URL.Scheme = req.Header.Get(X_LANTERN_SCHEME)
			// Grab the actual host from the original client and use that for the outbound request
			req.URL.Host = req.Header.Get(X_LANTERN_HOST)
			req.Host = req.URL.Host
			log.Printf("Handling request for: %s", req.URL.String())
		},
	}
	rp.ServeHTTP(resp, req)
}

// initCerts initializes a private key and certificates, used both for the
// server HTTPS proxy and the client MITM proxy.  Both types of proxy have a CA
// certificate.  The server proxy also gets a server certificate signed by that
// CA.  When running as a client proxy, any newly generated CA certificate is
// added to the current user's trust store (e.g. keychain) as a trusted root.
func initCerts(host string) (err error) {
	if pk, err = keyman.LoadPKFromFile(PK_FILE); err != nil {
		if pk, err = keyman.GeneratePK(2048); err != nil {
			return
		}
		if err = pk.WriteToFile(PK_FILE); err != nil {
			return
		}
	}

	caCert, err = keyman.LoadCertificateFromFile(CA_CERT_FILE)
	if err != nil || caCert.X509().NotAfter.Before(ONE_MONTH_FROM_TODAY) {
		log.Println("Creating new self-signed CA cert")
		if caCert, err = certificateFor("Lantern", TEN_YEARS_FROM_TODAY, true, nil); err != nil {
			return
		}
		if err = caCert.WriteToFile(CA_CERT_FILE); err != nil {
			return
		}
		if isDownstream {
			log.Println("Adding issuing cert to user trust store as trusted root")
			if err = caCert.AddAsTrustedRoot(); err != nil {
				return
			}
		}
	}

	if isUpstream {
		serverCert, err = keyman.LoadCertificateFromFile(SERVER_CERT_FILE)
		if err != nil || caCert.X509().NotAfter.Before(ONE_MONTH_FROM_TODAY) {
			log.Println("Creating new server cert")
			if serverCert, err = certificateFor(host, ONE_YEAR_FROM_TODAY, true, caCert); err != nil {
				return
			}
			if err = serverCert.WriteToFile(SERVER_CERT_FILE); err != nil {
				return
			}
		}
	}
	return
}

// certificateFor generates a certificate for a given name, signed by the given
// issuer.  If no issuer is specified, the generated certificate is
// self-signed.
func certificateFor(
	name string,
	validUntil time.Time,
	isCA bool,
	issuer *keyman.Certificate) (cert *keyman.Certificate, err error) {
	template := &x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(int64(time.Now().Nanosecond())),
		Subject: pkix.Name{
			Organization: []string{"Lantern"},
			CommonName:   name,
		},
		NotBefore: time.Now().AddDate(0, -1, 0),
		NotAfter:  validUntil,

		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}
	if issuer == nil {
		if isCA {
			template.KeyUsage = template.KeyUsage | x509.KeyUsageCertSign
		}
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.IsCA = true
	}
	cert, err = pk.Certificate(template, issuer)
	return
}
