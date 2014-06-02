// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"

	"github.com/getlantern/enproxy"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/flashlight/proxy"
	"github.com/getlantern/keyman"
)

var (
	// Command-line Flags
	help         = flag.Bool("help", false, "Get usage help")
	addr         = flag.String("addr", "", "ip:port on which to listen for requests.  When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https")
	upstreamHost = flag.String("server", "", "hostname at which to connect to a server flashlight (always using https).  When specified, this flashlight will run as a client proxy, otherwise it runs as a server")
	upstreamPort = flag.Int("serverport", 443, "the port on which to connect to the server")
	masqueradeAs = flag.String("masquerade", "", "masquerade host: if specified, flashlight will actually make a request to this host's IP but with a host header corresponding to the 'server' parameter")
	rootCA       = flag.String("rootca", "", "pin to this CA cert if specified (PEM format)")
	configDir    = flag.String("configdir", "", "directory in which to store configuration (defaults to current directory)")
	instanceId   = flag.String("instanceid", "", "instanceId under which to report stats to statshub.  If not specified, no stats are reported.")
	dumpheaders  = flag.Bool("dumpheaders", false, "dump the headers of outgoing requests and responses to stdout")
	cpuprofile   = flag.String("cpuprofile", "", "write cpu profile to given file")

	// flagsParsed is unused, this is just a trick to allow us to parse
	// command-line flags before initializing the other variables
	flagsParsed = parseFlags()

	isDownstream = *upstreamHost != ""
	isUpstream   = !isDownstream
)

// parseFlags parses the command-line flags.  If there's a problem with the
// provided flags, it prints usage to stdout and exits with status 1.
func parseFlags() bool {
	flag.Parse()
	if *help || *addr == "" {
		flag.Usage()
		os.Exit(1)
	}
	return true
}

func main() {
	if *cpuprofile != "" {
		startCPUProfiling(*cpuprofile)
		stopCPUProfilingOnSigINT(*cpuprofile)
		defer stopCPUProfiling(*cpuprofile)
	}

	// Set up the common ProxyConfig for clients and servers
	proxyConfig := proxy.ProxyConfig{
		Addr:              *addr,
		ShouldDumpHeaders: *dumpheaders,
		ReadTimeout:       0, // don't timeout
		WriteTimeout:      0,
	}

	log.Debugf("Running proxy")
	if isDownstream {
		runClientProxy(proxyConfig)
	} else {
		runServerProxy(proxyConfig)
	}
}

// Runs the client-side proxy
func runClientProxy(proxyConfig proxy.ProxyConfig) {
	client := &proxy.Client{
		ProxyConfig: proxyConfig,
		EnproxyConfig: &enproxy.Config{
			DialProxy: func(addr string) (net.Conn, error) {
				return tls.Dial("tcp", addressForServer(), clientTLSConfig())
			},
			NewRequest: func(method string, body io.Reader) (req *http.Request, err error) {
				return http.NewRequest(method, "http://"+*upstreamHost+":80/", body)
			},
		},
	}
	err := client.Run()
	if err != nil {
		log.Fatalf("Unable to run client proxy: %s", err)
	}
}

// Runs the server-side proxy
func runServerProxy(proxyConfig proxy.ProxyConfig) {
	useAllCores()
	server := &proxy.Server{
		ProxyConfig: proxyConfig,
		InstanceId:  *instanceId,
		CertContext: &proxy.CertContext{
			PKFile:         inConfigDir("proxypk.pem"),
			ServerCertFile: inConfigDir("servercert.pem"),
		},
	}
	err := server.Run()
	if err != nil {
		log.Fatalf("Unable to run server proxy: %s", err)
	}
}

// Get the address to dial for reaching the server
func addressForServer() string {
	serverHost := *upstreamHost
	if *masqueradeAs != "" {
		serverHost = *masqueradeAs
	}
	return fmt.Sprintf("%s:%d", serverHost, *upstreamPort)
}

// Build a tls.Config for the client to use in dialing server
func clientTLSConfig() *tls.Config {
	tlsConfig := &tls.Config{}
	if *rootCA != "" {
		caCert, err := keyman.LoadCertificateFromPEMBytes([]byte(*rootCA))
		if err != nil {
			log.Fatalf("Unable to load root ca cert: %s", err)
		}
		tlsConfig.RootCAs = caCert.PoolContainingCert()
	}
	return tlsConfig
}

// inConfigDir returns the path to the given filename inside of the configDir
// specified at the command line.
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

func useAllCores() {
	numcores := runtime.NumCPU()
	log.Debugf("Using all %d cores on machine", numcores)
	runtime.GOMAXPROCS(numcores)
}

func startCPUProfiling(filename string) {
	f, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	pprof.StartCPUProfile(f)
	log.Debugf("Process will save cpu profile to %s after terminating", filename)
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
	log.Debugf("Saving CPU profile to: %s", filename)
	pprof.StopCPUProfile()
}
