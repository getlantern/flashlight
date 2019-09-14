package diagnostics

import (
	"fmt"
	"io"
	"net"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"

	"github.com/getlantern/errors"
)

// Warning: do not set to >= 1 second: https://github.com/google/gopacket/issues/499
const packetReadTimeout = 100 * time.Millisecond

type capturedPacket struct {
	data []byte
	info gopacket.CaptureInfo
}

type captureProcess struct {
	addr            string
	buffer          *byteSliceRingMap
	iface           *networkInterface
	captureInfo     map[time.Time]gopacket.CaptureInfo
	captureInfoLock sync.Mutex
	stopChan        chan struct{}
}

// startCapture for the input address, saving packets to the provided buffer. Non-blocking.
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
		addr:            addr,
		buffer:          buffer,
		iface:           iface,
		captureInfo:     map[time.Time]gopacket.CaptureInfo{},
		captureInfoLock: sync.Mutex{},
		stopChan:        make(chan struct{}),
	}

	go func() {
		for {
			select {
			case pkt := <-pktSrc:
				ts := pkt.Metadata().Timestamp
				pid := getPktID(addr, ts)
				onDelete := func() {
					proc.captureInfoLock.Lock()
					delete(proc.captureInfo, ts)
					proc.captureInfoLock.Unlock()
				}
				err := proc.buffer.put(pid, pkt.Data(), onDelete)
				if err != nil {
					log.Errorf("failed to write packet to capture buffer: %v", err)
					continue
				}
				proc.captureInfoLock.Lock()
				proc.captureInfo[ts] = pkt.Metadata().CaptureInfo
				proc.captureInfoLock.Unlock()
			case <-proc.stopChan:
				handle.Close()
				return
			}
		}
	}()

	return &proc, nil
}

func (cp *captureProcess) stop() {
	close(cp.stopChan)
}

// Packets are not returned sorted.
func (cp *captureProcess) capturedSince(t time.Time) []capturedPacket {
	cp.captureInfoLock.Lock()
	defer cp.captureInfoLock.Unlock()

	capturedSince := []capturedPacket{}
	for timestamp, ci := range cp.captureInfo {
		if t.After(timestamp) {
			continue
		}
		if pktData, ok := cp.buffer.get(getPktID(cp.addr, timestamp)); ok {
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
//
// Captured packets are saved in a ring buffer and may be overwritten by newly captured packets. At
// any time, a group of packets can be saved by calling SaveCaptures. These saved captures can then
// be written out in pcapng format using WritePcapng.
type TrafficLog struct {
	captureBuffer *byteSliceRingMap
	savedCaptures *byteSliceRingMap
	captureInfo   map[string]captureInfo
	captureProcs  map[string]*captureProcess

	// Protects savedCaptures, captureInfo, and captureProcs. The captureBuffer is written to by
	// captureProcesses which do not respect this lock.
	mu sync.Mutex
}

// NewTrafficLog returns a new TrafficLog. Start capture by calling UpdateAddresses.
//
// Captured packets will be saved in a fixed-size ring buffer, the size of which is specified by
// captureBytes. At any time, a group of captured packets can be saved by calling SaveCaptures. This
// moves packets captured for a specified address and time period into a separate fixed-size ring
// buffer. The size of this buffer is specified by saveBytes.
func NewTrafficLog(captureBytes, saveBytes int) *TrafficLog {
	return &TrafficLog{
		newByteSliceRingMap(captureBytes),
		newByteSliceRingMap(saveBytes),
		map[string]captureInfo{},
		map[string]*captureProcess{},
		sync.Mutex{},
	}
}

// UpdateAddresses updates the addresses for which traffic is being captured. Capture will begin (or
// continue) for all addresses in the input slice. Capture will be stopped for any addresses not in
// the input slice.
//
// If an error is returned, the addresses have not been updated. In other words, a partial update is
// not possible.
func (tl *TrafficLog) UpdateAddresses(addresses []string) error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	newCaptureProcs := []*captureProcess{}
	stopAllNewCaptures := func() {
		for _, proc := range newCaptureProcs {
			proc.stop()
		}
	}

	captureProcs := map[string]*captureProcess{}
	for _, addr := range addresses {
		if proc, ok := tl.captureProcs[addr]; ok {
			captureProcs[addr] = proc
		} else {
			proc, err := startCapture(addr, tl.captureBuffer)
			if err != nil {
				stopAllNewCaptures()
				return errors.New("failed to start capture for %s: %v", addr, err)
			}
			captureProcs[addr] = proc
			newCaptureProcs = append(newCaptureProcs, proc)
		}
	}
	for addr, proc := range tl.captureProcs {
		if _, ok := captureProcs[addr]; !ok {
			proc.stop()
		}
	}
	tl.captureProcs = captureProcs
	return nil
}

// SaveCaptures saves all captures for the given address received in the past duration d. These
// captured packets will be moved from the main capture buffer into a fixed-size ring buffer
// specifically for saved captures. Saved packets will only be overwritten upon future calls to
// SaveCaptures.
func (tl *TrafficLog) SaveCaptures(address string, d time.Duration) error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	proc, ok := tl.captureProcs[address]
	if !ok {
		// Not really an error as it's possible the capture process has simply stopped.
		return nil
	}
	captures := proc.capturedSince(time.Now().Add(-1 * d))
	before := func(i, j int) bool { return captures[i].info.Timestamp.Before(captures[j].info.Timestamp) }
	sort.Slice(captures, before)

	var (
		numErrors int
		lastError error
	)
	for _, capture := range captures {
		pid := getPktID(address, capture.info.Timestamp)
		err := tl.savedCaptures.put(pid, capture.data, func() { delete(tl.captureInfo, pid) })
		if err != nil {
			numErrors++
			lastError = err
		} else {
			tl.captureInfo[pid] = captureInfo{capture.info, proc.iface}
		}
	}
	if numErrors > 0 {
		return errors.New("%d errors saving packets; last error: %v", numErrors, lastError)
	}
	return nil
}

// WritePcapng writes saved captures in pcapng file format.
func (tl *TrafficLog) WritePcapng(w io.Writer) error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	// If other link types are needed, they will be added to the writer in calls to AddInterface.
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
		ci, ok := tl.captureInfo[pktID]
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

// Close the TrafficLog. All captures will stop and the log will be cleared.
func (tl *TrafficLog) Close() error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	for _, proc := range tl.captureProcs {
		proc.stop()
	}
	tl.captureBuffer = nil
	tl.savedCaptures = nil
	tl.captureInfo = nil
	tl.captureProcs = nil
	return nil
}

func getPktID(addr string, t time.Time) string {
	return fmt.Sprintf("%s-%s", addr, t.Format(time.StampMicro))
}
