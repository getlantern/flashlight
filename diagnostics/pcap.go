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

	// Maps indices to IDs for network interfaces. Indices are supplied by the operating system. IDs
	// are assigned by the pcapgo package upon registration of the interface.
	interfaceIndicesToIDs map[int]int

	// Keys are the IDs assigned by the pcapgo package.
	interfaceStats map[int]*pcapgo.NgInterfaceStatistics
}

func newPcapNgWriter(w io.Writer) (*pcapngWriter, error) {
	// If other link types are needed, they will be added to the writer in calls to addInterface.
	underlying, err := pcapgo.NewNgWriter(w, layers.LinkTypeEthernet)
	if err != nil {
		return nil, err
	}
	return &pcapngWriter{
		sync.Mutex{}, underlying, map[int]int{}, map[int]*pcapgo.NgInterfaceStatistics{},
	}, nil
}

func (w *pcapngWriter) registerInterface(iface networkInterface) error {
	w.Lock()
	defer w.Unlock()

	if _, ok := w.interfaceIndicesToIDs[iface.netInterface.Index]; ok {
		return nil
	}

	id, err := w.w.AddInterface(pcapgo.NgInterface{
		Name:        iface.name(),
		Description: iface.pcapInterface.Description,
		OS:          runtime.GOOS,
		LinkType:    iface.linkType,
		SnapLength:  uint32(iface.mtu()),
	})
	if err != nil {
		return err
	}

	w.interfaceIndicesToIDs[iface.index()] = id
	w.interfaceStats[id] = &pcapgo.NgInterfaceStatistics{StartTime: time.Now()}
	return nil
}

func (w *pcapngWriter) writePacket(pkt gopacket.Packet) error {
	w.Lock()
	defer w.Unlock()

	ci := pkt.Metadata().CaptureInfo
	ifaceID, ok := w.interfaceIndicesToIDs[ci.InterfaceIndex]
	if !ok {
		return errors.New("received packet for interface %d, but this interface is unregistered", ci.InterfaceIndex)
	}

	// Oddly, the pcapgo package actually expects this to be the ID assigned upon registration.
	ci.InterfaceIndex = ifaceID

	stats, ok := w.interfaceStats[ifaceID]
	if !ok {
		w.interfaceStats[ifaceID] = &pcapgo.NgInterfaceStatistics{StartTime: time.Now()}
	}

	if err := w.w.WritePacket(ci, pkt.Data()); err != nil {
		stats.PacketsDropped++
		return err
	}
	stats.PacketsReceived++
	return nil
}

func (w *pcapngWriter) flush() error {
	w.Lock()
	defer w.Unlock()

	for id, stats := range w.interfaceStats {
		stats.LastUpdate = time.Now()
		stats.EndTime = time.Now()
		w.w.WriteInterfaceStats(id, *stats)
	}
	return w.w.Flush()
}

// CaptureProxyTraffic captures all traffic for the proxies in the input map. Capture stops and the
// function returns when a signal is sent on cfg.StopChannel (or the channel is closed). The output
// will be in pcapng format.
//
// If an error is returned, it will be of type ErrorsMap. The keys of the map will be the keys in
// proxiesMap. If no error occurred for a given proxy, it will have no entry in the returned map.
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
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("malformed address: %v", err)
	}

	remoteIPs, err := net.LookupIP(host)
	if err != nil {
		errors.New("failed to find IP for host: %v", err)
	}
	if len(remoteIPs) < 1 {
		errors.New("failed to resolve host")
	}
	remoteIP := remoteIPs[0]

	iface, err := networkInterfaceFor(remoteIP)
	if err != nil {
		return errors.New("failed to obtain interface: %v", err)
	}

	if err := w.registerInterface(*iface); err != nil {
		return errors.New("failed to write interface information to output: %v", err)
	}

	handle, err := pcap.OpenLive(iface.pcapName(), int32(iface.mtu()), false, packetReadTimeout)
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
		fmt.Sprintf("%s dst %v and dst port %s", network, remoteIP, port),
		fmt.Sprintf("%s src %v and src port %s", network, remoteIP, port),
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
			}
		case <-stop:
			if len(pktWriteErrors) == 0 {
				return nil
			}
			return errors.New("%d errors writing packets; first error: %v", len(pktWriteErrors), pktWriteErrors[0])
		}
	}
}
