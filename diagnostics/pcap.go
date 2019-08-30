package diagnostics

import (
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/chained"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
)

const (
	// DefaultCaptureDuration is the default duration for packet capture sessions.
	DefaultCaptureDuration = 10 * time.Second

	// Warning: do not set to >= 1 second: https://github.com/google/gopacket/issues/499
	packetReadTimeout = 100 * time.Millisecond
)

// ErrorsMap represents multiple errors. ErrorsMap implements the error interface.
type ErrorsMap map[string]error

func (em ErrorsMap) Error() string {
	keys := []string{}
	for k := range em {
		keys = append(keys, k)
	}
	return fmt.Sprintf("errors for %s", strings.Join(keys, ", "))
}

// CaptureConfig is configuration for a packet capture session.
type CaptureConfig struct {
	// Defaults to CloseAfter(DefaultCaptureDuration).
	StopChannel <-chan struct{}

	// Defaults to os.Stdout.
	Output io.Writer

	setStopChannelOnce sync.Once
}

func (cc *CaptureConfig) getStopChannel() <-chan struct{} {
	if cc.StopChannel == nil {
		cc.setStopChannelOnce.Do(func() {
			cc.StopChannel = CloseAfter(DefaultCaptureDuration)
		})
	}
	return cc.StopChannel
}

func (cc *CaptureConfig) getOutput() io.Writer {
	if cc.Output == nil {
		return os.Stdout
	}
	return cc.Output
}

// CloseAfter returns a channel which will close after the specified duration.
func CloseAfter(d time.Duration) <-chan struct{} {
	c, ready := make(chan struct{}), make(chan struct{})
	go func() {
		close(ready)
		<-time.After(d)
		close(c)
	}()
	<-ready
	return c
}

type pcapngWriter struct {
	sync.Mutex
	w *pcapgo.NgWriter
}

func newPcapNgWriter(w io.Writer) (*pcapngWriter, error) {
	// If other link types are needed, they will be added to the writer in calls to addInterface.
	underlying, err := pcapgo.NewNgWriter(w, layers.LinkTypeEthernet)
	if err != nil {
		return nil, err
	}
	return &pcapngWriter{sync.Mutex{}, underlying}, nil
}

func (w pcapngWriter) addInterface(iface pcapgo.NgInterface) (id int, err error) {
	w.Lock()
	defer w.Unlock()
	return w.w.AddInterface(iface)
}

func (w pcapngWriter) writePacket(p gopacket.Packet) error {
	w.Lock()
	defer w.Unlock()
	return w.w.WritePacket(p.Metadata().CaptureInfo, p.Data())
}

func (w pcapngWriter) writeInterfaceStats(id int, stats pcapgo.NgInterfaceStatistics) error {
	w.Lock()
	defer w.Unlock()
	return w.w.WriteInterfaceStats(id, stats)
}

func (w pcapngWriter) flush() error {
	w.Lock()
	defer w.Unlock()
	return w.w.Flush()
}

// CaptureProxyTraffic generates a pcap file for each proxy in the input map. These files are saved
// in outputDir and named using the keys in proxiesMap.
//
// If an error is returned, it will be of type ErrorsMap. The keys of the map will be the keys in
// proxiesMap. If no error occurred for a given proxy, it will have no entry in the returned map.
//
// Expects tshark to be installed, otherwise returns errors.
func CaptureProxyTraffic(proxiesMap map[string]*chained.ChainedServerInfo, cfg *CaptureConfig) error {
	type captureError struct {
		proxyName string
		err       error
	}

	w, err := newPcapNgWriter(cfg.getOutput())
	if err != nil {
		return errors.New("failed to initialize pcapng writer: %v", err)
	}

	wg := new(sync.WaitGroup)
	captureErrors := make(chan captureError, len(proxiesMap))
	for proxyName, proxyInfo := range proxiesMap {
		wg.Add(1)
		go func(pName, pAddr string) {
			defer wg.Done()
			err := captureAndWrite(pAddr, w, cfg.getStopChannel())
			if err != nil {
				captureErrors <- captureError{pName, err}
			}
		}(proxyName, proxyInfo.Addr)
	}
	wg.Wait()
	close(captureErrors)
	w.flush()

	errorsMap := ErrorsMap{}
	for capErr := range captureErrors {
		errorsMap[capErr.proxyName] = capErr.err
	}
	if len(errorsMap) == 0 {
		return nil
	}
	return errorsMap
}

func captureAndWrite(addr string, w *pcapngWriter, stop <-chan struct{}) error {
	remoteHost, remotePort, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("malformed address: %v", err)
	}

	remoteIPs, err := net.LookupIP(remoteHost)
	if err != nil {
		return errors.New("failed to find IP for host: %v", err)
	}
	if len(remoteIPs) < 1 {
		return errors.New("failed to resolve host")
	}
	remoteIP := remoteIPs[0]

	localIP, err := preferredOutboundIP(remoteIP)
	if err != nil {
		return errors.New("failed to obtain outbound IP: %v", err)
	}

	iface, err := networkInterfaceFor(localIP)
	if err != nil {
		return errors.New("failed to obtain interface: %v", err)
	}

	linkType := layers.LinkTypeEthernet
	if remoteIP.IsLoopback() && runtime.GOOS != "linux" {
		// This is done to support testing.
		linkType = layers.LinkTypeNull
	}

	ifaceID, err := w.addInterface(pcapgo.NgInterface{
		Name:        iface.Name,
		Description: iface.Description,
		OS:          runtime.GOOS,
		LinkType:    linkType,
		SnapLength:  uint32(iface.mtu),
	})
	if err != nil {
		return errors.New("failed to write interface information to output: %v", err)
	}
	ifaceStats := pcapgo.NgInterfaceStatistics{StartTime: time.Now()}

	handle, err := pcap.OpenLive(iface.Name, int32(iface.mtu), false, packetReadTimeout)
	if err != nil {
		return errors.New("failed to open capture handle: %v", err)
	}
	defer handle.Close()

	network := "ip"
	if remoteIP.To4() == nil {
		network = "ip6"
	}
	bpf := fmt.Sprintf(
		"(%s) or (%s)",
		fmt.Sprintf("%s dst %v and dst port %s", network, remoteIP, remotePort),
		fmt.Sprintf("%s src %v and src port %s", network, remoteIP, remotePort),
	)
	if err := handle.SetBPFFilter(bpf); err != nil {
		return errors.New("failed to set capture filter: %v", err)
	}

	layerType := layers.LayerTypeEthernet
	if remoteIP.IsLoopback() && runtime.GOOS != "linux" {
		// This is done to support testing.
		layerType = layers.LayerTypeLoopback
	}

	pktSrc := gopacket.NewPacketSource(handle, layerType).Packets()
	pktWriteErrors := []error{}
	for {
		select {
		case pkt := <-pktSrc:
			if err := w.writePacket(pkt); err != nil {
				pktWriteErrors = append(pktWriteErrors, err)
				ifaceStats.PacketsDropped++
			} else {
				ifaceStats.PacketsReceived++
			}
		case <-stop:
			ifaceStats.LastUpdate = time.Now()
			ifaceStats.EndTime = time.Now()
			w.writeInterfaceStats(ifaceID, ifaceStats)
			if len(pktWriteErrors) == 0 {
				return nil
			}
			return errors.New("%d errors writing packets; first error: %v", len(pktWriteErrors), pktWriteErrors[0])
		}
	}
}

func preferredOutboundIP(remoteIP net.IP) (net.IP, error) {
	// Note: the choice of port below does not actually matter. It just needs to be non-zero.
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: remoteIP, Port: 999})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP, nil
}
