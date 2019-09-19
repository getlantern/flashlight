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

type captureInfo struct {
	// gopacket.CaptureInfo
	unixNano                              int64
	captureLength, length, interfaceIndex int
	iface                                 *networkInterface
}

func newCaptureInfo(pkt gopacket.Packet, iface *networkInterface) captureInfo {
	pktCI := pkt.Metadata().CaptureInfo
	return captureInfo{
		pktCI.Timestamp.UnixNano(), pktCI.CaptureLength, pktCI.Length, pktCI.InterfaceIndex, iface,
	}
}

func (ci captureInfo) gopacketCI() gopacket.CaptureInfo {
	return gopacket.CaptureInfo{
		Timestamp:      time.Unix(0, ci.unixNano),
		CaptureLength:  ci.captureLength,
		Length:         ci.length,
		InterfaceIndex: ci.interfaceIndex,
	}
}

type capturedPacket struct {
	data []byte
	info captureInfo
}

type captureProcess struct {
	addr   string
	buffer *byteSliceRingMap
	iface  *networkInterface
	// captureInfo     map[time.Time]gopacket.CaptureInfo
	captureInfo     map[int64]captureInfo // keys are unix nano timestamps
	captureInfoLock sync.Mutex
	stopChan        chan struct{}
}

type packetID struct {
	addr     string
	unixNano int64
}

func (pid packetID) String() string {
	return fmt.Sprintf("%s-%d", pid.addr, pid.unixNano)
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
		captureInfo:     map[int64]captureInfo{},
		captureInfoLock: sync.Mutex{},
		stopChan:        make(chan struct{}),
	}

	go func() {
		for {
			select {
			case pkt := <-pktSrc:
				ts := pkt.Metadata().Timestamp.UnixNano()
				pid := packetID{proc.addr, ts}
				onDelete := func() {
					proc.captureInfoLock.Lock()
					delete(proc.captureInfo, ts)
					proc.captureInfoLock.Unlock()
				}
				err := proc.buffer.put(pid.String(), pkt.Data(), onDelete)
				if err != nil {
					log.Errorf("failed to write packet to capture buffer: %v", err)
					continue
				}
				proc.captureInfoLock.Lock()
				proc.captureInfo[ts] = newCaptureInfo(pkt, proc.iface)
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
	for _, ci := range cp.captureInfo {
		if t.UnixNano() > ci.unixNano {
			continue
		}
		if pktData, ok := cp.buffer.get(packetID{cp.addr, ci.unixNano}.String()); ok {
			capturedSince = append(capturedSince, capturedPacket{pktData, ci})
		}
	}
	return capturedSince
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
	proc, ok := tl.captureProcs[address]
	tl.mu.Unlock()
	if !ok {
		// Not really an error as it's possible the capture process has simply stopped.
		return nil
	}
	captures := proc.capturedSince(time.Now().Add(-1 * d))
	before := func(i, j int) bool { return captures[i].info.unixNano < captures[i].info.unixNano }
	sort.Slice(captures, before)

	var (
		numErrors int
		lastError error
	)
	for _, capture := range captures {
		pid := packetID{address, capture.info.unixNano}.String()
		onDelete := func() {
			tl.mu.Lock()
			delete(tl.captureInfo, pid)
			tl.mu.Unlock()
		}
		err := tl.savedCaptures.put(pid, capture.data, onDelete)
		if err != nil {
			numErrors++
			lastError = err
		} else {
			tl.mu.Lock()
			tl.captureInfo[pid] = capture.info
			tl.mu.Unlock()
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
		gopacketCI := ci.gopacketCI()
		// Oddly, the pcapgo package expects this to be the registration ID.
		gopacketCI.InterfaceIndex = id
		if err := pcapW.WritePacket(gopacketCI, pktData); err != nil {
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
