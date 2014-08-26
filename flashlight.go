// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"

	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/flashlight/proxy"
	"github.com/getlantern/flashlight/statreporter"
	"github.com/getlantern/flashlight/statserver"
	"github.com/getlantern/go-igdman/igdman"
)

const (
	PORTMAP_FAILURE = 50
)

var (
	// Command-line Flags
	help         = flag.Bool("help", false, "Get usage help")
	addr         = flag.String("addr", "", "ip:port on which to listen for requests. When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https (required)")
	portmap      = flag.Int("portmap", 0, "try to map this port on the firewall to the port on which flashlight is listening, using UPnP or NAT-PMP. If mapping this port fails, flashlight will exit with status code 50")
	role         = flag.String("role", "", "either 'client' or 'server' (required)")
	upstreamHost = flag.String("server", "", "FQDN of flashlight server (required)")
	upstreamPort = flag.Int("serverport", 443, "the port on which to connect to the server")
	masqueradeAs = flag.String("masquerade", "", "masquerade host: if specified, flashlight will actually make a request to this host's IP but with a host header corresponding to the 'server' parameter")
	rootCA       = flag.String("rootca", "", "pin to this CA cert if specified (PEM format)")
	configDir    = flag.String("configdir", "", "directory in which to store configuration (defaults to current directory)")
	instanceId   = flag.String("instanceid", "", "instanceId under which to report stats to statshub. If not specified, no stats are reported.")
	statsAddr    = flag.String("statsaddr", "", "host:port at which to make detailed stats available using server-sent events (optional)")
	country      = flag.String("country", "xx", "2 digit country code under which to report stats. Defaults to xx.")
	dumpheaders  = flag.Bool("dumpheaders", false, "dump the headers of outgoing requests and responses to stdout")
	cpuprofile   = flag.String("cpuprofile", "", "write cpu profile to given file")
	memprofile   = flag.String("memprofile", "", "write heap profile to given file")
	parentPID    = flag.Int("parentpid", 0, "the parent process's PID, used on Windows for killing flashlight when the parent disappears")

	// flagsParsed is unused, this is just a trick to allow us to parse
	// command-line flags before initializing the other variables
	flagsParsed = parseFlags()

	isDownstream = *role == "client"
	isUpstream   = !isDownstream
)

// parseFlags parses the command-line flags.  If there's a problem with the
// provided flags, it prints usage to stdout and exits with status 1.
func parseFlags() bool {
	flag.Parse()
	if *help || *addr == "" || (*role != "server" && *role != "client") || *upstreamHost == "" {
		flag.Usage()
		os.Exit(1)
	}
	return true
}

func main() {
	if *cpuprofile != "" {
		startCPUProfiling(*cpuprofile)
		defer stopCPUProfiling(*cpuprofile)
	}

	if *memprofile != "" {
		defer saveMemProfile(*memprofile)
	}

	saveProfilingOnSigINT()

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
		ProxyConfig:  proxyConfig,
		UpstreamHost: *upstreamHost,
		UpstreamPort: *upstreamPort,
		MasqueradeAs: *masqueradeAs,
		RootCA:       *rootCA,
	}
	err := client.Run()
	if err != nil {
		log.Fatalf("Unable to run client proxy: %s", err)
	}
}

// Runs the server-side proxy
func runServerProxy(proxyConfig proxy.ProxyConfig) {
	useAllCores()

	if *portmap > 0 {
		log.Debugf("Attempting to map external port %d", *portmap)
		err := mapPort()
		if err != nil {
			log.Errorf("Unable to map external port: %s", err)
			os.Exit(PORTMAP_FAILURE)
		}
		log.Debugf("Mapped external port %d", *portmap)
	}

	server := &proxy.Server{
		ProxyConfig: proxyConfig,
		Host:        *upstreamHost,
		CertContext: &proxy.CertContext{
			PKFile:         inConfigDir("proxypk.pem"),
			ServerCertFile: inConfigDir("servercert.pem"),
		},
	}
	if *instanceId != "" {
		// Report stats
		server.StatReporter = &statreporter.Reporter{
			InstanceId: *instanceId,
			Country:    *country,
		}
	}
	if *statsAddr != "" {
		// Serve stats
		server.StatServer = &statserver.Server{
			Addr: *statsAddr,
		}
	}
	err := server.Run()
	if err != nil {
		log.Fatalf("Unable to run server proxy: %s", err)
	}
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

func mapPort() error {
	parts := strings.Split(*addr, ":")

	internalPort, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("Unable to parse local port: ")
	}

	internalIP := parts[0]
	if internalIP == "" {
		internalIP, err = determineInternalIP()
		if err != nil {
			return fmt.Errorf("Unable to determine internal IP: %s", err)
		}
	}

	igd, err := igdman.NewIGD()
	if err != nil {
		return fmt.Errorf("Unable to get IGD: %s", err)
	}

	igd.RemovePortMapping(igdman.TCP, *portmap)
	err = igd.AddPortMapping(igdman.TCP, internalIP, internalPort, *portmap, 0)
	if err != nil {
		return fmt.Errorf("Unable to map port with igdman %d: %s", *portmap, err)
	}

	return nil
}

func determineInternalIP() (string, error) {
	conn, err := net.Dial("tcp", "s3.amazonaws.com:443")
	if err != nil {
		return "", fmt.Errorf("Unable to determine local IP: %s", err)
	}
	defer conn.Close()
	return strings.Split(conn.LocalAddr().String(), ":")[0], nil
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

func stopCPUProfiling(filename string) {
	log.Debugf("Saving CPU profile to: %s", filename)
	pprof.StopCPUProfile()
}

func saveMemProfile(filename string) {
	f, err := os.Create(filename)
	if err != nil {
		log.Errorf("Unable to create file to save memprofile: %s", err)
		return
	}
	log.Debugf("Saving heap profile to: %s", filename)
	pprof.WriteHeapProfile(f)
	f.Close()
}

func saveProfilingOnSigINT() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		if *cpuprofile != "" {
			stopCPUProfiling(*cpuprofile)
		}
		if *memprofile != "" {
			saveMemProfile(*memprofile)
		}
		os.Exit(2)
	}()
}
