package trafficlog

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"github.com/oxtoacart/bpool"

	"github.com/getlantern/errors"
)

// Overhead (in bytes) per packet. This figure is calculated empirically using the opp utility.
var overheadPerPacket = 160

// SetMeasurementMode is used by test utilities to take measurements on traffic logs.
func SetMeasurementMode(on bool) {
	overheadPerPacket = 0
}

type captureInfo struct {
	unixNano                              int64
	captureLength, length, interfaceIndex int
	iface                                 *networkInterface
}

func newCaptureInfo(ci gopacket.CaptureInfo, iface *networkInterface) captureInfo {
	return captureInfo{
		ci.Timestamp.UnixNano(), ci.CaptureLength, ci.Length, ci.InterfaceIndex, iface,
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
	info     captureInfo
	dataBuf  *bytes.Buffer
	dataPool *bpool.BufferPool
}

// size in bytes.
func (pkt capturedPacket) size() int {
	return pkt.dataBuf.Len() + overheadPerPacket
}

func (pkt capturedPacket) onEvict() {
	pkt.dataPool.Put(pkt.dataBuf)
}

type captureProcess struct {
	buffer    *sharedBufferHook
	dataPool  *bpool.BufferPool
	errorChan chan error
	statsChan chan CaptureStats
	stopChan  chan struct{}
	readGroup *sync.WaitGroup
	f         MutatorFactory
}

// startCapture for the input address, saving packets to the provided buffer. Non-blocking.
func startCapture(
	addr string, buffer *sharedBufferHook, dataPool *bpool.BufferPool,
	f MutatorFactory, mtuLimit int, statsInterval time.Duration) (*captureProcess, error) {

	proc := captureProcess{
		buffer:    buffer,
		dataPool:  dataPool,
		errorChan: make(chan error),
		statsChan: make(chan CaptureStats),
		stopChan:  make(chan struct{}),
		readGroup: new(sync.WaitGroup),
		f:         f,
	}
	initErr := make(chan error)
	go proc.watchRoutes(addr, mtuLimit, statsInterval, initErr)
	if err := <-initErr; err != nil {
		return nil, err
	}
	return &proc, nil
}

func (cp *captureProcess) watchRoutes(
	addr string, mtuLimit int, statsInterval time.Duration, initErr chan error) {

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		initErr <- errors.New("malformed address: %v", err)
		close(initErr)
		return
	}

	startRouteCapture := func(u routeUpdate) (stopChan chan struct{}, err error) {
		if u.iface.mtu() > mtuLimit {
			return nil, errors.New("interface MTU (%d) exceeds the MTU limit (%d)", u.iface.mtu(), mtuLimit)
		}

		handle, err := pcap.OpenLive(u.iface.pcapName(), int32(u.iface.mtu()), false, packetReadTimeout)
		if err != nil {
			return nil, errors.New("failed to open capture handle: %v", err)
		}

		network := "ip"
		if u.ip.To4() == nil {
			network = "ip6"
		}
		bpf := fmt.Sprintf(
			"(%s) or (%s)",
			fmt.Sprintf("%s dst %v and dst port %s", network, u.ip, port),
			fmt.Sprintf("%s src %v and src port %s", network, u.ip, port),
		)
		if err := handle.SetBPFFilter(bpf); err != nil {
			handle.Close()
			return nil, errors.New("failed to set capture filter: %v", err)
		}

		stopChan = make(chan struct{})
		go cp.readPackets(handle, u.iface, statsInterval, stopChan)
		return stopChan, nil
	}

	rw, err := newRouteWatcher(host)
	if err != nil {
		initErr <- errors.New("failed to establish route to host: %v", err)
		close(initErr)
		return
	}

	// If the first update causes an error, there's probably something wrong - fail fast.
	t := capturedRouteTracker{make(map[string]capturedRoute), startRouteCapture}
	if err := t.handleUpdate(<-rw.updates()); err != nil {
		initErr <- err
		close(initErr)
		return
	}
	close(initErr)

	for {
		select {
		case u := <-rw.updates():
			if err := t.handleUpdate(u); err != nil {
				cp.logError(err)
			}
		case <-cp.stopChan:
			t.close()
			rw.close()
			return
		}
	}
}

func (cp *captureProcess) readPackets(
	handle *pcap.Handle, iface networkInterface, statsInterval time.Duration, stopChan <-chan struct{}) {

	var received, droppedByUs uint64
	mutator := cp.f.MutatorFor(iface.linkType)
	statsTimer := time.NewTimer(statsInterval)
	cp.readGroup.Add(1)
	go func() {
		defer cp.readGroup.Done()
		defer statsTimer.Stop()
		for {
			select {
			case <-statsTimer.C:
				cp.logStats(handle, received, droppedByUs)
				statsTimer.Reset(statsInterval)
			default:
			}

			data, ci, err := handle.ZeroCopyReadPacketData()
			if err != nil && err == io.EOF {
				return
			}
			if err != nil {
				if nextErr, ok := err.(pcap.NextError); !ok || nextErr != pcap.NextErrorTimeoutExpired {
					cp.logError(errors.New("failed to read packet from capture handle: %v", err))
					droppedByUs++
				}
				continue
			}

			dataBuf := cp.dataPool.Get()
			if err = mutator(data, dataBuf); err != nil {
				cp.logError(errors.New("packet mutation error: %v", err))
				cp.dataPool.Put(dataBuf)
				droppedByUs++
				continue
			}
			ci.CaptureLength = dataBuf.Len()
			cp.buffer.put(capturedPacket{newCaptureInfo(ci, &iface), dataBuf, cp.dataPool})
			received++
		}
	}()
	<-stopChan
	cp.logStats(handle, received, droppedByUs)
	handle.Close()
}

func (cp *captureProcess) logError(err error) {
	select {
	case cp.errorChan <- err:
	default:
	}
}

// logStats should not be called concurrently with any other methods on h.
func (cp *captureProcess) logStats(h *pcap.Handle, received, droppedByUs uint64) {
	// The "received" packets in the handle stats include all packets the handle saw on the
	// interface (pre-BPF). We ignore that, but the dropped statistics reflect packets we might have
	// missed because we weren't keeping up with ingress.
	stats, err := h.Stats()
	if err != nil {
		cp.logError(errors.New("failed to read capture stats: %v", err))
		return
	}
	cs := CaptureStats{received, uint64(stats.PacketsDropped) + droppedByUs}
	select {
	case cp.statsChan <- cs:
	default:
	}
}

func (cp *captureProcess) stop() {
	close(cp.stopChan)
	cp.readGroup.Wait()
	close(cp.statsChan)
	close(cp.errorChan)
	cp.buffer.close()
}

// forEach applies the input function to all packets currently in the buffer. Packets will be
// provided in the order in which they were captured. Capture will be blocked while this function
// runs so care should be taken to ensure the input function is cheap. The data in each packet's
// buffer is only valid while this function runs. Once the function returns, these buffers may be
// overwritten.
func (cp *captureProcess) forEach(do func(pkt capturedPacket)) {
	cp.buffer.forEach(func(i bufferItem) {
		do(i.(capturedPacket))
	})
}

type capturedRoute struct {
	iface    networkInterface
	stopChan chan struct{}
}

type capturedRouteTracker struct {
	// Maps IP addresses to routes currently being captured.
	routes       map[string]capturedRoute
	startCapture func(u routeUpdate) (stopChan chan struct{}, err error)
}

func (t *capturedRouteTracker) handleUpdate(u routeUpdate) error {
	ipStr := u.ip.String()
	if r, ok := t.routes[ipStr]; ok && r.iface.name() == u.iface.name() {
		return nil
	}

	stopChan, err := t.startCapture(u)
	if err != nil {
		return err
	}
	if r, ok := t.routes[ipStr]; ok {
		close(r.stopChan)
		delete(t.routes, ipStr)
	}
	t.routes[ipStr] = capturedRoute{u.iface, stopChan}
	return nil
}

func (t *capturedRouteTracker) close() {
	for _, r := range t.routes {
		close(r.stopChan)
	}
}
