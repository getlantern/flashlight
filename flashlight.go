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
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/getlantern/go-mitm/mitm"
	"github.com/oxtoacart/keyman"
)

const (
	CONNECT                  = "CONNECT"
	X_LANTERN_REQUEST_INFO   = "X-Lantern-Request-Info"
	X_LANTERN_PUBLIC_IP      = "X-LANTERN-PUBLIC-IP"
	SESSIONS_TO_CACHE_CLIENT = 10000
	SESSIONS_TO_CACHE_SERVER = 100000

	HR = "--------------------------------------------------------------------------------"
)

var (
	help         = flag.Bool("help", false, "Get usage help")
	addr         = flag.String("addr", "", "ip:port on which to listen for requests.  When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https")
	upstreamHost = flag.String("server", "", "hostname at which to connect to a server flashlight (always using https).  When specified, this flashlight will run as a client proxy, otherwise it runs as a server")
	upstreamPort = flag.Int("serverport", 443, "the port on which to connect to the server")
	masqueradeAs = flag.String("masquerade", "", "masquerade host: if specified, flashlight will actually make a request to this host's IP but with a host header corresponding to the 'server' parameter")
	configDir    = flag.String("configdir", "", "directory in which to store configuration (defaults to current directory)")
	instanceId   = flag.String("instanceid", "", "instanceId under which to report stats to statshub.  If not specified, no stats are reported.")
	dumpheaders  = flag.Bool("dumpheaders", false, "dump the headers of outgoing requests and responses to stdout")
	cpuprofile   = flag.String("cpuprofile", "", "write cpu profile to given file")

	// flagsParsed is unused, this is just a trick to allow us to parse
	// command-line flags before initializing the other variables
	flagsParsed = parseFlags()

	TOMORROW             = time.Now().AddDate(0, 0, 1)
	ONE_MONTH_FROM_TODAY = time.Now().AddDate(0, 1, 0)
	ONE_YEAR_FROM_TODAY  = time.Now().AddDate(1, 0, 0)
	TEN_YEARS_FROM_TODAY = time.Now().AddDate(10, 0, 0)

	shouldReportStats = *instanceId != ""
	isDownstream      = *upstreamHost != ""
	isUpstream        = !isDownstream

	// CloudFlare based protocol
	cp = newCloudFlareClientProtocol(*upstreamHost, *upstreamPort, *masqueradeAs)
	sp = newCloudFlareServerProtocol()

	clientProxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Check for local addresses, which we don't rewrite
			ip, err := net.ResolveIPAddr("ip4", strings.Split(req.Host, ":")[0])
			if err == nil && !ip.IP.IsLoopback() {
				cp.rewriteRequest(req)
			}
			dumpHeaders("Request", req.Header)
		},
		Transport: withRewrite(cp.rewriteResponse, &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return cp.dial(addr)
			},
			TLSClientConfig: &tls.Config{
				// Use a TLS session cache to minimize TLS connection establishment
				// Requires Go 1.3+
				ClientSessionCache: tls.NewLRUClientSessionCache(SESSIONS_TO_CACHE_CLIENT),
			},
		}),
	}

	serverProxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			sp.rewriteRequest(req)
			log.Printf("Handling request for: %s", req.URL.String())
			dumpHeaders("Request", req.Header)
		},
		Transport: withRewrite(sp.rewriteResponse, &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				conn, err := net.Dial(network, addr)
				if err != nil {
					return nil, err
				}
				if shouldReportStats {
					// When reporting stats, use a special connection that counts bytes
					return &countingConn{conn}, nil
				}
				return conn, err
			},
			TLSClientConfig: &tls.Config{
				// Use a TLS session cache to minimize TLS connection establishment
				// Requires Go 1.3+
				ClientSessionCache: tls.NewLRUClientSessionCache(SESSIONS_TO_CACHE_SERVER),
			},
		}),
	}

	PK_FILE          = inConfigDir("proxypk.pem")
	CA_CERT_FILE     = inConfigDir("cacert.pem")
	SERVER_CERT_FILE = inConfigDir("servercert.pem")

	pk                 *keyman.PrivateKey
	caCert, serverCert *keyman.Certificate

	wg sync.WaitGroup
)

func parseFlags() bool {
	flag.Parse()
	if *help || *addr == "" {
		flag.Usage()
		os.Exit(1)
	}
	return true
}

func inConfigDir(filename string) string {
	if *configDir == "" {
		return filename
	} else {
		if _, err := os.Stat(*configDir); err != nil {
			if os.IsNotExist(err) {
				// Create config dir
				if err := os.MkdirAll(*configDir, 0755); err != nil {
					log.Fatalf("Unable to create configDir at %s: %s", *configDir, err)
				}
			}
		}
		return fmt.Sprintf("%s%c%s", *configDir, os.PathSeparator, filename)
	}
}

func main() {
	if *cpuprofile != "" {
		startCPUProfiling(*cpuprofile)
		stopCPUProfilingOnSigINT(*cpuprofile)
		defer stopCPUProfiling(*cpuprofile)
	}

	if err := initCerts(strings.Split(*addr, ":")[0]); err != nil {
		log.Fatalf("Unable to initialize certs: %s", err)
	}
	if isDownstream {
		runClient()
	} else {
		useAllCores()
		if shouldReportStats {
			iid := *instanceId
			log.Printf("Reporting stats under instanceId %s", iid)
			startReportingStats(iid)
		} else {
			log.Println("Not reporting stats (no instanceId specified at command line)")
		}
		runServer()
	}
	wg.Wait()
}

// runClient runs the client HTTP proxy server
func runClient() {
	wg.Add(1)

	mitmHandler := buildMITMHandler()

	server := &http.Server{
		Addr:         *addr,
		Handler:      mitmHandler,
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

// buildMITMHandler builds the MITM handler that the client uses for proxying
// HTTPS requests. We have to MITM these because we can't CONNECT tunnel through
// CloudFlare.
func buildMITMHandler() http.Handler {
	cryptoConf := &mitm.CryptoConfig{
		PKFile:   PK_FILE,
		CertFile: CA_CERT_FILE,
	}
	mitmHandler, err := mitm.Wrap(clientProxy, cryptoConf)
	if err != nil {
		log.Fatalf("Unable to initialize mitm proxy: %s", err)
	}
	return mitmHandler
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

// handleServer handles requests to the server-side (upstream) proxy
func handleServer(resp http.ResponseWriter, req *http.Request) {
	if req.Header.Get(X_LANTERN_REQUEST_INFO) != "" {
		handleInfoRequest(resp, req)
	} else {
		// Proxy as usual
		serverProxy.ServeHTTP(resp, req)
	}
}

// handleInfoRequest looks up info about the client (right now just ip address)
// and returns it to the client
func handleInfoRequest(resp http.ResponseWriter, req *http.Request) {
	// Client requested their info
	clientIp := req.Header.Get("X-Forwarded-For")
	if clientIp == "" {
		clientIp = strings.Split(req.RemoteAddr, ":")[0]
	} else {
		// X-Forwarded-For may contain multiple ips, use the last
		ips := strings.Split(clientIp, ",")
		clientIp = ips[len(ips)-1]
	}
	resp.Header().Set(X_LANTERN_PUBLIC_IP, clientIp)
	resp.WriteHeader(200)
}

func hostIncludingPort(req *http.Request) (host string) {
	host = req.Host
	if !strings.Contains(host, ":") {
		host = host + ":443"
	}
	return
}

func respondBadGateway(connIn net.Conn, msg string) {
	connIn.Write([]byte(fmt.Sprintf("HTTP/1.1 502 Bad Gateway: %s", msg)))
	connIn.Close()
}

// initCerts initializes a private key and certificates, used both for the
// server HTTPS proxy and the client MITM proxy.  Both types of proxy have a CA
// certificate.  The server proxy also gets a server certificate signed by that
// CA.  When running as a client proxy, any newly generated CA certificate is
// added to the current user's trust store (e.g. keychain) as a trusted root.
func initCerts(host string) (err error) {
	if pk, err = keyman.LoadPKFromFile(PK_FILE); err != nil {
		log.Printf("Creating new PK at: %s", PK_FILE)
		if pk, err = keyman.GeneratePK(2048); err != nil {
			return
		}
		if err = pk.WriteToFile(PK_FILE); err != nil {
			return
		}
	}

	caCert, err = keyman.LoadCertificateFromFile(CA_CERT_FILE)
	if err != nil || caCert.X509().NotAfter.Before(ONE_MONTH_FROM_TODAY) {
		log.Printf("Creating new self-signed CA cert at: %s", CA_CERT_FILE)
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
			log.Printf("Creating new server cert at: %s", SERVER_CERT_FILE)
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

func useAllCores() {
	numcores := runtime.NumCPU()
	log.Printf("Using all %d cores on machine", numcores)
	runtime.GOMAXPROCS(numcores)
}

func startCPUProfiling(filename string) {
	f, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	pprof.StartCPUProfile(f)
	log.Printf("Process will save cpu profile to %s after terminating: %s")
}

func stopCPUProfilingOnSigINT(filename string) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		stopCPUProfiling(filename)
		os.Exit(0)
	}()
}

func stopCPUProfiling(filename string) {
	log.Printf("Saving CPU profile to: %s", filename)
	pprof.StopCPUProfile()
}

// writhRewrite creates a RoundTripper that uses the supplied RoundTripper and
// rewrites the response.
func withRewrite(rw func(*http.Response), rt http.RoundTripper) http.RoundTripper {
	return &wrappedRoundTripper{
		rewrite: rw,
		orig:    rt,
	}
}

type wrappedRoundTripper struct {
	rewrite func(*http.Response)
	orig    http.RoundTripper
}

func (rt *wrappedRoundTripper) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	resp, err = rt.orig.RoundTrip(req)
	if err == nil {
		rt.rewrite(resp)
	}
	dumpHeaders("Response", resp.Header)
	return
}

func dumpHeaders(category string, headers http.Header) {
	if *dumpheaders {
		log.Printf("%s\n%s\n%s\n%s", category, HR, spew.Sdump(headers), HR)
	}
}
