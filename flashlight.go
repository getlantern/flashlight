// flashlight is a lightweight chained proxy that can run in client or server mode.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"

	"github.com/getlantern/flashlight/impl"
	"github.com/getlantern/flashlight/protocol/cloudflare"
)

var (
	// Command-line Flags
	help             = flag.Bool("help", false, "Get usage help")
	addr             = flag.String("addr", "", "ip:port on which to listen for requests.  When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https")
	upstreamHost     = flag.String("server", "", "hostname at which to connect to a server flashlight (always using https).  When specified, this flashlight will run as a client proxy, otherwise it runs as a server")
	upstreamPort     = flag.Int("serverport", 443, "the port on which to connect to the server")
	masqueradeAs     = flag.String("masquerade", "", "masquerade host: if specified, flashlight will actually make a request to this host's IP but with a host header corresponding to the 'server' parameter")
	masqueradeCACert = flag.String("masqueradecacert", "", "pin to this CA cert if specified (PEM format)")
	configDir        = flag.String("configdir", "", "directory in which to store configuration (defaults to current directory)")
	instanceId       = flag.String("instanceid", "", "instanceId under which to report stats to statshub.  If not specified, no stats are reported.")
	dumpheaders      = flag.Bool("dumpheaders", false, "dump the headers of outgoing requests and responses to stdout")
	cpuprofile       = flag.String("cpuprofile", "", "write cpu profile to given file")
	install          = flag.Bool("install", false, "install prerequisites into environment and then terminate")

	// flagsParsed is unused, this is just a trick to allow us to parse
	// command-line flags before initializing the other variables
	flagsParsed = parseFlags()

	isDownstream = *upstreamHost != ""
)

// parseFlags parses the command-line flags.  If there's a problem with the
// provided flags, it prints usage to stdout and exits with status 1.
func parseFlags() bool {
	flag.Parse()
	if (*help || *addr == "") && !*install {
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
	proxyConfig := impl.ProxyConfig{
		Addr:              *addr,
		ShouldDumpHeaders: *dumpheaders,
		ReadTimeout:       0, // don't timeout
		WriteTimeout:      0,
		CertContext: &impl.CertContext{
			PKFile:         inConfigDir("proxypk.pem"),
			CACertFile:     inConfigDir("cacert.pem"),
			ServerCertFile: inConfigDir("servercert.pem"),
		},
	}

	var proxy impl.Proxy
	if isDownstream {
		// Protocol is right now hardcoded to use CloudFlare, could be made
		// configurable to support other protocols like Fastly.
		protocol, err := cloudflare.NewClientProtocol(*upstreamHost, *upstreamPort, *masqueradeAs, *masqueradeCACert)
		if err != nil {
			log.Fatalf("Error initializing CloudFlare client protocol: %s", err)
			os.Exit(1)
		}
		proxy = &impl.Client{
			ProxyConfig:  proxyConfig,
			UpstreamHost: *upstreamHost,
			Protocol:     protocol,
		}
	} else {
		protocol := cloudflare.NewServerProtocol()
		proxy = &impl.Server{
			ProxyConfig: proxyConfig,
			Protocol:    protocol,
			InstanceId:  *instanceId,
		}
		useAllCores()
	}

	if *install {
		log.Println("Installing proxy")
		err := proxy.Install()
		if err != nil {
			log.Fatalf("Unable to install proxy: %s", err)
		}
	} else {
		log.Println("Running proxy")
		err := proxy.Run()
		if err != nil {
			log.Fatalf("Unable to run proxy: %s", err)
		}
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
	log.Printf("Process will save cpu profile to %s after terminating", filename)
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
