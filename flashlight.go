// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	//"crypto/tls"
	//"bufio"
	"crypto/x509"
	"crypto/x509/pkix"
	"flag"
	"fmt"
	"io"
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
	"github.com/oxtoacart/keyman"
)

const (
	CONNECT = "CONNECT"

	ONE_WEEK  = 7 * 24 * time.Hour
	TWO_WEEKS = ONE_WEEK * 2

	X_LANTERN_HOST   = "X-Lantern-Host"
	X_LANTERN_METHOD = "X-Lantern-Method"

	PK_FILE   = "proxypk.pem"
	CERT_FILE = "proxycert.pem"
)

var (
	OK_RESPONSE    = []byte("HTTP/1.1 200 Connection established\r\n")
	END_OF_HEADERS = []byte("\r\n")

	help         = flag.Bool("help", false, "Get usage help")
	addr         = flag.String("addr", "", "ip:port on which to listen for requests.  When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https")
	upstreamHost = flag.String("server", "", "hostname at which to connect to a server flashlight (always using https).  When specified, this flashlight will run as a client proxy, otherwise it runs as a server")
	upstreamPort = flag.Int("serverPort", 80, "the port on which to connect to the server")
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
		// if err := server.ListenAndServeTLS(CERT_FILE, PK_FILE); err != nil {
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Unable to start server proxy: %s", err)
		}
		wg.Done()
	}()
}

// handleClient handles requests from a local client (e.g. the browser)
func handleClient(resp http.ResponseWriter, req *http.Request) {
	log.Println(spew.Sdump(req))

	host := *upstreamHost
	if *masqueradeAs != "" {
		host = *masqueradeAs
	}
	upstreamAddr := fmt.Sprintf("%s:%d", host, *upstreamPort)

	// Remember the host that was actually requested
	req.Header.Set(X_LANTERN_HOST, req.Host)
	// Set our upstream proxy as the host for this request
	req.Host = *upstreamHost
	// TODO: handle https too
	req.URL.Scheme = "http"
	req.Header.Set("Host", *upstreamHost)
	req.URL.Host = *upstreamHost
	// To allow tunneling through reverse proxies that don't support CONNECT,
	// pretend to be a GET request.
	if req.Method == "CONNECT" {
		req.Method = "POST"
		req.Header.Set(X_LANTERN_METHOD, "CONNECT")
		//req.Header.Set("Transfer-Encoding", "chunked")
	}

	log.Printf("Using %s to handle request for: %s", upstreamAddr, req.URL.String())
	// serverIsLocalhost := strings.Contains(*addr, "localhost") || strings.Contains(*addr, "127.0.0.1")
	// tlsConfig := &tls.Config{
	// 	ServerName:         host,
	// 	InsecureSkipVerify: serverIsLocalhost,
	// }
	// connOut, err := tls.Dial("tcp", upstreamAddr, tlsConfig)
	connOut, err := net.Dial("tcp", upstreamAddr)
	if err != nil {
		if connOut != nil {
			defer connOut.Close()
		}
		msg := fmt.Sprintf("Unable to dial server: %s", err)
		log.Println(msg)
		respondBadGateway(resp, req, msg)
	} else {
		go req.WriteProxy()
		go func() {
			defer connOut.Close()
			io.Copy(resp, connOut)
		}()
	}
}

// handleServer handles requests from a downstream flashlight
func handleServer(resp http.ResponseWriter, req *http.Request) {
	// Grab the actual host from the original client and use that for the outbound request
	req.URL.Host = req.Header.Get(X_LANTERN_HOST)
	req.Host = req.URL.Host
	if "CONNECT" == req.Header.Get(X_LANTERN_METHOD) {
		// Request is actually a CONNECT pretending to be a POST
		handleServerHTTPS(resp, req)
	} else {
		handleServerHTTP(resp, req)
	}
}

// handleServerHTTP handles http requests from a downstream flashlight
func handleServerHTTP(resp http.ResponseWriter, req *http.Request) {
	rp := httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			log.Printf("Handling request for: %s", req.URL.String())
		},
	}
	rp.ServeHTTP(resp, req)
}

// handleServerHTTPS handles HTTPS requests from a downstream flashlight
func handleServerHTTPS(resp http.ResponseWriter, req *http.Request) {
	if connIn, _, err := resp.(http.Hijacker).Hijack(); err != nil {
		msg := fmt.Sprintf("Unable to access underlying connection from client: %s", err)
		respondBadGateway(resp, req, msg)
	} else {
		connOut, err := net.Dial("tcp", hostIncludingPort(req))
		if err != nil {
			if connOut != nil {
				defer connOut.Close()
			}
			msg := fmt.Sprintf("Unable to dial server: %s", err)
			log.Println(msg)
			respondBadGateway(resp, req, msg)
		} else {
			log.Printf("Tunneling traffic")
			pipe(connIn, connOut)
			connIn.Write(OK_RESPONSE)
			connIn.Write([]byte("Content-Length: 1000000000\r\n"))
			_, err = connIn.Write(END_OF_HEADERS)
		}
	}
}

func writeRequest(req *http.Request, conn io.Writer) error {
	if _, err := fmt.Fprintf(conn, "%s %s HTTP/1.1\r\n", valueOrDefault(req.Method, "GET"), req.URL.RequestURI()); err != nil {
		return err
	}
	err := req.Header.Write(conn)
	if err != nil {
		return err
	}
	_, err = conn.Write([]byte("Transfer-Encoding: chunked\r\n"))
	if err != nil {
		return err
	}
	_, err = conn.Write([]byte("\r\n"))
	_, err = conn.Write([]byte("0"))
	_, err = conn.Write([]byte(""))
	return err
}

func valueOrDefault(value, def string) string {
	if value != "" {
		return value
	}
	return def
}

func hostIncludingPort(req *http.Request) (host string) {
	host = req.Host
	if !strings.Contains(host, ":") {
		if req.Method == "CONNECT" {
			host = host + ":443"
		} else {
			host = host + ":80"
		}
	}
	return
}

func respondBadGateway(resp http.ResponseWriter, req *http.Request, msg string) {
	resp.WriteHeader(502)
	resp.Write([]byte(fmt.Sprintf("Bad Gateway: %s - %s", req.URL, msg)))
}

func pipe(connIn net.Conn, connOut net.Conn) {
	go func() {
		defer connIn.Close()
		io.Copy(connOut, connIn)
	}()
	go func() {
		defer connOut.Close()
		io.Copy(connIn, connOut)
	}()
}

func pipeChunked(connIn net.Conn, connOut net.Conn) {
	go func() {
		defer connIn.Close()
		io.Copy(httputil.NewChunkedWriter(connOut), connIn)
	}()
	go func() {
		defer connOut.Close()
		io.Copy(connIn, httputil.NewChunkedReader(connOut))
	}()
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
		if issuingCert, err = certificateFor("localhost", nil); err != nil {
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
