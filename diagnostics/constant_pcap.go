package diagnostics

import (
	"fmt"
	"io"
	"net"
	"runtime"
	"time"

	"github.com/getlantern/errors"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
)

type capturedPacket struct {
	data []byte
	info gopacket.CaptureInfo
}

// TODO: needs to be concurrency safe
type captureProcess struct {
	addr           string
	buffer         *byteSliceRingMap
	iface          *networkInterface
	captureInfoMap map[time.Time]gopacket.CaptureInfo
	stopChan       chan struct{}
}

func startCapture(addr string, buffer *byteSliceRingMap) (*captureProcess, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, errors.New("malformed address: %v", err)
	}

	remoteIPs, err := net.LookupIP(host)
	if err != nil {
		return nil, errors.New("failed to find IP for host: %v", err)
	}
	if len(remoteIPs) < 1 {
		return nil, errors.New("failed to resolve host")
	}
	remoteIP := remoteIPs[0]

	iface, err := networkInterfaceFor(remoteIP)
	if err != nil {
		return nil, errors.New("failed to obtain interface: %v", err)
	}

	handle, err := pcap.OpenLive(iface.pcapName(), int32(iface.mtu()), false, packetReadTimeout)
	if err != nil {
		return nil, errors.New("failed to open capture handle: %v", err)
	}

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
		handle.Close()
		return nil, errors.New("failed to set capture filter: %v", err)
	}

	layerType := layers.LayerTypeEthernet
	if remoteIP.IsLoopback() && runtime.GOOS != "linux" {
		// This is done to support testing.
		layerType = layers.LayerTypeLoopback
	}
	pktSrc := gopacket.NewPacketSource(handle, layerType).Packets()

	proc := captureProcess{
		addr:           addr,
		buffer:         buffer,
		iface:          iface,
		captureInfoMap: map[time.Time]gopacket.CaptureInfo{},
		stopChan:       make(chan struct{}),
	}

	go func() {
		for {
			select {
			case pkt := <-pktSrc:
				ts := pkt.Metadata().Timestamp
				pid := getPktID(addr, ts)
				err := proc.buffer.put(pkt.Data(), pid, func() { delete(proc.captureInfoMap, ts) })
				if err != nil {
					log.Errorf("failed to write packet to capture buffer: %v", err)
					continue
				}
				proc.captureInfoMap[ts] = pkt.Metadata().CaptureInfo
			case <-proc.stopChan:
				handle.Close()
			}
		}
	}()

	return &proc, nil
}

func (cp captureProcess) stop() {
	close(cp.stopChan)
}

func (cp captureProcess) capturedSince(t time.Time) []capturedPacket {
	capturedSince := []capturedPacket{}
	for timestamp, ci := range cp.captureInfoMap {
		if !timestamp.After(t) {
			continue
		}
		if pktData, ok := cp.buffer.get(getPktID(cp.addr, t)); ok {
			capturedSince = append(capturedSince, capturedPacket{pktData, ci})
		}
	}
	return capturedSince
}

type captureInfo struct {
	gopacket.CaptureInfo
	iface *networkInterface
}

// TrafficLog is a log of network traffic.
// TODO: document ring-buffer mechanics of captured traffic and saved captures
// TODO: determine if these methods need to be concurrency safe
type TrafficLog struct {
	captureBuffer *byteSliceRingMap
	savedCaptures *byteSliceRingMap
	captureInfos  map[string]captureInfo
	captureProcs  map[string]captureProcess
}

// NewTrafficLog returns a new TrafficLog. Start capture by calling UpdateAddresses.
func NewTrafficLog(addresses []string) (*TrafficLog, error) {
	// TODO: implement me!
	return nil, nil
}

// UpdateAddresses updates the addresses for which traffic is being captured. Capture will begin (or
// continue) for all addresses in the input slice. Capture will be stopped for any addresses not in
// the input slice.
func (tl *TrafficLog) UpdateAddresses(addresses []string) error {
	newCaptureProcs := map[string]captureProcess{}
	stopAllNewCaptures := func() {
		for _, proc := range newCaptureProcs {
			proc.stop()
		}
	}

	for _, addr := range addresses {
		if proc, ok := tl.captureProcs[addr]; ok {
			newCaptureProcs[addr] = proc
		} else {
			proc, err := startCapture(addr, tl.captureBuffer)
			if err != nil {
				stopAllNewCaptures()
				return err
			}
			newCaptureProcs[addr] = *proc
		}
	}
	for addr, proc := range tl.captureProcs {
		if _, ok := newCaptureProcs[addr]; !ok {
			proc.stop()
		}
	}
	tl.captureProcs = newCaptureProcs
	return nil
}

// SaveCaptures saves all captures for the given address received in the past duration d.
func (tl *TrafficLog) SaveCaptures(address string, d time.Duration) error {
	proc, ok := tl.captureProcs[address]
	if !ok {
		// Not really an error as it's possible the capture process has simply stopped.
		return nil
	}
	captures := proc.capturedSince(time.Now().Add(-1 * d))

	var (
		numErrors int
		lastError error
	)
	for _, capture := range captures {
		pktID := getPktID(address, capture.info.Timestamp)
		err := tl.savedCaptures.put(capture.data, pktID, func() { delete(tl.captureInfos, pktID) })
		if err != nil {
			numErrors++
			lastError = err
		} else {
			tl.captureInfos[pktID] = captureInfo{capture.info, proc.iface}
		}
	}
	if numErrors > 0 {
		return errors.New("%d errors saving packets; last error: %v", lastError)
	}
	return nil
}

// WritePcapng writes saved captures in pcapng file format.
func (tl *TrafficLog) WritePcapng(w io.Writer) error {
	// If other link types are needed, they will be added to the writer in calls to addInterface.
	pcapW, err := pcapgo.NewNgWriter(w, layers.LinkTypeEthernet)
	if err != nil {
		return errors.New("failed to initialize pcapng writer: %v", err)
	}

	// maps networkInterface.index() -> registration IDs
	interfaceIDs := map[int]int{}
	registerInterface := func(iface *networkInterface) (id int, err error) {
		if id, ok := interfaceIDs[iface.index()]; ok {
			return id, nil
		}
		id, err = pcapW.AddInterface(pcapgo.NgInterface{
			Name:        iface.name(),
			Description: iface.pcapInterface.Description,
			OS:          runtime.GOOS,
			LinkType:    iface.linkType,
			SnapLength:  uint32(iface.mtu()),
		})
		if err == nil {
			interfaceIDs[iface.index()] = id
		}
		return
	}

	var (
		numErrors int
		lastError error
	)
	// TODO: do we need the statistics?
	tl.savedCaptures.forEach(func(pktID string, pktData []byte) {
		ci, ok := tl.captureInfos[pktID]
		if !ok {
			numErrors++
			lastError = errors.New("could not find capture info for packet")
			return
		}
		id, err := registerInterface(ci.iface)
		if err != nil {
			numErrors++
			lastError = errors.New("failed to register interface: %v", err)
			return
		}
		// Oddly, the pcapgo package expects this to be the registration ID.
		ci.InterfaceIndex = id
		if err := pcapW.WritePacket(ci.CaptureInfo, pktData); err != nil {
			numErrors++
			lastError = errors.New("failed to write packet: %v", err)
			return
		}
	})
	if numErrors > 0 {
		return errors.New("%d errors writing packets; last error: %v", numErrors, lastError)
	}
	if err := pcapW.Flush(); err != nil {
		return errors.New("failed to flush writer: %v", err)
	}
	return nil
}

func getPktID(addr string, t time.Time) string {
	return fmt.Sprintf("%s-%s", addr, t.Format(time.RFC3339))
}
