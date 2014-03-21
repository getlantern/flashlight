// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"flag"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/oxtoacart/keyman"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"sync"
	"time"
)

const (
	CONNECT        = "CONNECT"
	ONE_WEEK       = 7 * 24 * time.Hour
	TWO_WEEKS      = ONE_WEEK * 2
	X_LANTERN_HOST = "X-Lantern-Host"

	PK_FILE   = "proxypk.pem"
	CERT_FILE = "proxycert.pem"
)

var (
	help         = flag.Bool("help", false, "Get usage help")
	addr         = flag.String("addr", "", "ip:port on which to listen for requests.  When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https")
	upstreamHost = flag.String("server", "", "hostname at which to connect to a server flashlight (always using https).  When specified, this flashlight will run as a client proxy, otherwise it runs as a server")
	upstreamPort = flag.Int("serverPort", 443, "the port on which to connect to the server")
	masqueradeAs = flag.String("masquerade", "", "masquerade host: if specified, flashlight will actually make a request to this host's IP but with a host header corresponding to the 'server' parameter")

	isDownstream bool

	pk             *keyman.PrivateKey
	pkPem          []byte
	issuingCert    *keyman.Certificate
	issuingCertPem []byte

	wg sync.WaitGroup
)

func init() {
	flag.Parse()
	if *help || *addr == "" {
		flag.Usage()
		os.Exit(1)
	}

	isDownstream = *upstreamHost != ""
}

func main() {
	if isDownstream {
		runClient()
	} else {
		if err := initCerts(); err != nil {
			log.Fatalf("Unable to initialize certs: %s", err)
		}
		runServer()
	}
	wg.Wait()
}

func runClient() {
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
		if err := server.ListenAndServeTLS(CERT_FILE, PK_FILE); err != nil {
			log.Fatalf("Unable to start server proxy: %s", err)
		}
		wg.Done()
	}()
}

// handleClient handles requests from a local client (e.g. the browser)
func handleClient(resp http.ResponseWriter, req *http.Request) {
	host := *upstreamHost
	if *masqueradeAs != "" {
		host = *masqueradeAs
	}
	upstreamAddr := fmt.Sprintf("%s:%d", host, *upstreamPort)

	rp := httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Remember the host that was actually requested
			req.Header.Set(X_LANTERN_HOST, req.Host)
			// Set our upstream proxy as the host for this request
			req.Host = *upstreamHost
			req.URL.Host = *upstreamHost
		},
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				tlsConfig := &tls.Config{
					ServerName: host,
				}
				log.Printf("Dialing %s", upstreamAddr)
				return tls.Dial(network, upstreamAddr, tlsConfig)
			},
		},
	}
	rp.ServeHTTP(resp, req)
}

// handleServer handles requests from a downstream flashlight
func handleServer(resp http.ResponseWriter, req *http.Request) {
	rp := httputil.ReverseProxy{
		Director: func(req *http.Request) {
			log.Printf("Request: %s", spew.Sdump(req))
			// TODO - need to add support for tunneling HTTPS traffic using CONNECT
			req.URL.Scheme = "http"
			// Grab the actual host from the original client and use that for the outbound request
			req.URL.Host = req.Header.Get(X_LANTERN_HOST)
			req.Host = req.URL.Host
		},
	}
	rp.ServeHTTP(resp, req)
}

func initCerts() (err error) {
	if pk, err = keyman.LoadPKFromFile(PK_FILE); err != nil {
		if pk, err = keyman.GeneratePK(2048); err != nil {
			return
		}
		if err = pk.WriteToFile(PK_FILE); err != nil {
			return
		}
	}
	pkPem = pk.PEMEncoded()
	if issuingCert, err = keyman.LoadCertificateFromFile(CERT_FILE); err != nil {
		if issuingCert, err = certificateFor("Lantern", nil); err != nil {
			return
		}
		if err = issuingCert.WriteToFile(CERT_FILE); err != nil {
			return
		}
	}
	issuingCertPem = issuingCert.PEMEncoded()
	return
}

func certificateFor(name string, issuer *keyman.Certificate) (cert *keyman.Certificate, err error) {
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(int64(time.Now().Nanosecond())),
		Subject: pkix.Name{
			Organization: []string{"Lantern"},
			CommonName:   name,
		},
		NotBefore: now.Add(-1 * ONE_WEEK),
		NotAfter:  now.Add(TWO_WEEKS),

		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}
	if issuer == nil {
		template.KeyUsage = template.KeyUsage | x509.KeyUsageCertSign
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.IsCA = true
	}
	cert, err = pk.Certificate(template, issuer)
	return
}
