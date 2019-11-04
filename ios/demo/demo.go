// This demo program allows testing the iOS packet forwarding functionality
// on a desktop machine using a TUN device. There are two ways to run it.
//
// To fetch configuration from the cloud, just like iOS does, run:
//
// ./demo -gw 192.168.1.1 -bypassthreads 100
//
// Replace 192.168.1.1 with your default gateway (here and below as well).
//
// The -bypassthreads flag will enable automatic configuration of the routing
// table to bypass the demo TUN device for traffic to your proxy as well as
// domain fronting traffic.
//
// Alternately, to point at a specific proxies.yaml, run:
//
// ./demo -gw 192.168.1.1 -proxiesyaml ~/proxies.yaml
//
// To have the demo program handle all your internet traffic, run:
//
// sudo route delete default
// sudo route add default 10.0.0.2
//
// If using a proxies.yaml, you'll also need to manually set up a direct route
// for proxy traffic via the default gateway, like so:
//
// sudo route add 67.205.172.79 192.168.1.1
//
// Now your network traffic will route through here to your proxy.
//
// When you're finished, you can fix your routing table with:
//
// sudo route delete default
// sudo route add default 102.168.1.1
//
// If you added a manual route for the proxy, you'll want to remove that too:
//
// sudo route delete 67.205.172.79
//
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/gotun"
	"github.com/getlantern/uuid"

	"github.com/getlantern/flashlight/ios"
)

var (
	log = golog.LoggerFor("ios-demo")
)

const (
	mtu = 65535
)

var (
	tunDevice       = flag.String("tun-device", "tun0", "tun device name")
	tunAddr         = flag.String("tun-address", "10.0.0.2", "tun device address")
	tunMask         = flag.String("tun-mask", "255.255.255.0", "tun device netmask")
	tunGW           = flag.String("tun-gw", "10.0.0.1", "tun device gateway")
	pprofAddr       = flag.String("pprofaddr", "", "pprof address to listen on, not activate pprof if empty")
	internetGateway = flag.String("gw", "192.168.1.1", "gateway for getting to Internet")
	deviceID        = flag.String("deviceid", base64.StdEncoding.EncodeToString(uuid.NodeID()), "deviceid to report to server")
	bypassThreads   = flag.Int("bypassthreads", 0, "number of threads to use for configuring bypass routes. If set to 0, we don't bypass.")
	proxiesYaml     = flag.String("proxiesyaml", "", "if specified, use the proxies.yaml at this location to configure client")
)

type fivetuple struct {
	proto            string
	srcIP, dstIP     string
	srcPort, dstPort int
}

func (ft fivetuple) String() string {
	return fmt.Sprintf("[%v] %v:%v -> %v:%v", ft.proto, ft.srcIP, ft.srcPort, ft.dstIP, ft.dstPort)
}

func main() {
	flag.Parse()

	if *pprofAddr != "" {
		go func() {
			log.Debugf("Starting pprof page at http://%s/debug/pprof", *pprofAddr)
			srv := &http.Server{
				Addr: *pprofAddr,
			}
			if err := srv.ListenAndServe(); err != nil {
				log.Error(err)
			}
		}()
	}

	tmpDir, err := ioutil.TempDir("", "ios_demo")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfgResult, err := ios.Configure(tmpDir, *deviceID)
	if err != nil {
		log.Fatal(err)
	}

	if *proxiesYaml != "" {
		log.Debugf("Using proxies.yaml at %v", *proxiesYaml)
		in, readErr := ioutil.ReadFile(*proxiesYaml)
		if readErr != nil {
			log.Fatal(readErr)
		}
		writeErr := ioutil.WriteFile(filepath.Join(tmpDir, "proxies.yaml"), in, 0644)
		if writeErr != nil {
			log.Fatal(writeErr)
		}
	}

	bbuffer := filepath.Join(tmpDir, "bordabuffer.bin")
	bbufferTemp := filepath.Join(tmpDir, "bordabuffer_temp.bin")
	if err := ios.ConfigureBorda("DEMO", 1, 10*time.Second, bbuffer, bbufferTemp); err != nil {
		log.Fatal(err)
	}

	go func() {
		// periodically report to borda
		for {
			time.Sleep(20 * time.Second)
			if err := ios.ReportToBorda(bbuffer); err != nil {
				log.Errorf("Unable to report to borda: %v", err)
			}
		}
	}()

	dev, err := tun.OpenTunDevice(*tunDevice, *tunAddr, *tunGW, *tunMask, 1500)
	if err != nil {
		log.Fatalf("Error opening TUN device: %v", err)
	}
	defer dev.Close()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGPIPE)
	go func() {
		<-ch
		log.Debug("Stopping TUN device")
		dev.Close()
		log.Debug("Stopped TUN device")
	}()

	doneAddingBypassRoutes := make(chan interface{})

	ipsToExclude := strings.Split(cfgResult.IPSToExcludeFromVPN, ",")
	if *bypassThreads > 0 {
		defer func() {
			<-doneAddingBypassRoutes
			log.Debugf("Deleting bypass routes for %d ips", len(ipsToExclude))

			ipsCh := make(chan string)
			var wg sync.WaitGroup
			wg.Add(*bypassThreads)
			for i := 0; i < *bypassThreads; i++ {
				go func() {
					for ip := range ipsCh {
						if deleteErr := exec.Command("sudo", "route", "delete", ip).Run(); deleteErr != nil {
							log.Errorf("Error deleting route fpr %v: %v", ip, deleteErr)
						}
					}
					wg.Done()
				}()
			}
			for i, ip := range ipsToExclude {
				ipsCh <- ip
				if i > 0 && i%50 == 0 {
					log.Debugf("Deleting bypass routes ... %d", i)
				}
			}
			close(ipsCh)
			wg.Wait()

			log.Debugf("Deleted bypass routes for %d ips", len(ipsToExclude))
		}()

		go func() {
			log.Debugf("Adding bypass routes for %d ips", len(ipsToExclude))

			ipsCh := make(chan string)
			var wg sync.WaitGroup
			wg.Add(*bypassThreads)
			for i := 0; i < *bypassThreads; i++ {
				go func() {
					for ip := range ipsCh {
						if addErr := exec.Command("sudo", "route", "add", ip, *internetGateway).Run(); addErr != nil {
							log.Error(addErr)
						}
					}
					wg.Done()
				}()
			}
			for i, ip := range ipsToExclude {
				ipsCh <- ip
				if i > 0 && i%50 == 0 {
					log.Debugf("Adding bypass routes ... %d", i)
				}
			}
			close(ipsCh)
			wg.Wait()

			log.Debugf("Added bypass routes for %d ips", len(ipsToExclude))
			close(doneAddingBypassRoutes)
		}()
	}

	writer, err := ios.Client(&writerAdapter{dev}, tmpDir, mtu)
	if err != nil {
		log.Fatal(err)
	}

	log.Debug("Reading from TUN device")
	b := make([]byte, mtu)
	for {
		n, err := dev.Read(b)
		if n > 0 {
			dataCap, _ := writer.Write(b[:n])
			if dataCap > 0 {
				log.Debugf("Data capped at %dMiB", dataCap)
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Errorf("Unexpected error reading from TUN device: %v", err)
			}
			return
		}
	}
}

type writerAdapter struct {
	Writer io.Writer
}

func (wa *writerAdapter) Write(b []byte) bool {
	_, err := wa.Writer.Write(b)
	return err == nil
}
