// This demo program allows testing the iOS packet forwarding functionality
// on a desktop machine.
//
// Run the program, then in a terminal
//
// sudo route delete default
// sudo route add default 10.0.0.2
// sudo route add 67.205.172.79 192.168.1.1
//
// Replace 192.168.1.1 with the IP of your default gateway
//
// Now your network traffic will route through here to the proxy at 67.205.172.79.
//
// When you're finished, you can fix your routing table with:
//
// sudo route delete default
// sudo route delete 67.205.172.79
// sudo route add default 102.168.1.1
//
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
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
	"github.com/getlantern/ipproxy"
	"github.com/getlantern/packetforward"
	"github.com/getlantern/uuid"

	"github.com/getlantern/flashlight/ios"
)

var (
	log = golog.LoggerFor("ios-demo")
)

var (
	tunDevice       = flag.String("tun-device", "tun0", "tun device name")
	tunAddr         = flag.String("tun-address", "10.0.0.2", "tun device address")
	tunMask         = flag.String("tun-mask", "255.255.255.0", "tun device netmask")
	tunGW           = flag.String("tun-gw", "10.0.0.1", "tun device gateway")
	ifOut           = flag.String("ifout", "en0", "name of interface to use for outbound connections")
	pprofAddr       = flag.String("pprofaddr", "", "pprof address to listen on, not activate pprof if empty")
	internetGateway = flag.String("gw", "192.168.1.1", "gateway for getting to Internet")
	deviceID        = flag.String("deviceid", base64.StdEncoding.EncodeToString(uuid.NodeID()), "deviceid to report to server")
	bypassThreads   = flag.Int("bypassthreads", 100, "number of threads to use for configuring bypass routes")
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

	l, err := net.Listen("tcp", ":3000")
	if err != nil {
		log.Fatal(err)
	}

	log.Debug("Starting packetforward server")
	pfs := packetforward.NewServer(&ipproxy.Opts{
		OutboundBufferDepth: 10000,
		DialTCP: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("tcp", "127.0.0.1:3000")
		},
		StatsInterval: 1 * time.Second,
	})
	go pfs.Serve(l)

	dev, err := tun.OpenTunDevice(*tunDevice, *tunAddr, *tunGW, *tunMask)
	if err != nil {
		log.Fatalf("Error opening TUN device: %v", err)
	}
	defer dev.Close()

	log.Debugf("Finding interface %v", *ifOut)
	outIF, err := net.InterfaceByName(*ifOut)
	if err != nil {
		log.Fatalf("Unable to find interface %v: %v", *ifOut, err)
	}
	outIFAddrs, err := outIF.Addrs()
	if err != nil {
		log.Fatalf("Unable to get addresses for interface %v: %v", *ifOut, err)
	}
	var laddrTCP *net.TCPAddr
	var laddrUDP *net.UDPAddr
	for _, outIFAddr := range outIFAddrs {
		switch t := outIFAddr.(type) {
		case *net.IPNet:
			ipv4 := t.IP.To4()
			if ipv4 != nil {
				laddrTCP = &net.TCPAddr{IP: ipv4, Port: 0}
				laddrUDP = &net.UDPAddr{IP: ipv4, Port: 0}
				break
			}
		}
	}
	if laddrTCP == nil {
		log.Fatalf("Unable to get IPv4 address for interface %v", *ifOut)
	}
	log.Debugf("Outbound TCP will use %v", laddrTCP)
	log.Debugf("Outbound UDP will use %v", laddrUDP)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		<-ch
		log.Debug("Stopping TUN device")
		dev.Close()
		log.Debug("Stopped TUN device")
	}()

	doneAddingBypassRoutes := make(chan interface{})

	ipsToExclude := strings.Split(cfgResult.IPSToExcludeFromVPN, ",")
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

	writer, err := ios.Client(&writerAdapter{dev}, tmpDir, 1500)
	if err != nil {
		log.Fatal(err)
	}

	log.Debug("Reading from TUN device")
	b := make([]byte, 1500)
	for {
		n, err := dev.Read(b)
		if n > 0 {
			writer.Write(b[:n])
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
