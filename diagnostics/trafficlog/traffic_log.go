// Package trafficlog provides a log for network traffic.
package trafficlog

import (
	"fmt"
	"io"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/oxtoacart/bpool"

	"github.com/getlantern/errors"
)

const (
	// Warning: do not set to >= 1 second: https://github.com/google/gopacket/issues/499
	packetReadTimeout = 100 * time.Millisecond

	// Size of buffered channels readable via public API.
	channelBufferSize = 10

	// Size of buffer pools used for packet data. The goal of the buffer pools is to avoid new
	// allocations through reuse. Another way to frame this goal is to avoid discarding buffers.
	// We size the buffer pools based on the ratio of the largest packets to the smallest packets.
	// The largest packets fill the standard MTU of 1500 bytes, while the smallest tend to be a
	// little over 50 bytes. When a 1500-byte packet arrives, it may evict up to 30 50-byte packets.
	// To avoid discarding the buffers, we therefore make the pools able to hold up to 30 buffers.
	dataPoolSize = 30

	// Capture processes report their stats to the traffic log more often than the traffic log
	// reports aggregated stats. This keeps the aggregated stats current.
	procStatsPerLogStats = 5
)

// DefaultMaxMTU is the default maximum acceptable MTU size of interfaces watched by traffic logs.
const DefaultMaxMTU = 1500

// MTULimitNone can be used to specify that there should be no limit to accepted MTU sizes.
const MTULimitNone = int(math.MaxInt32)

// DefaultStatsInterval is the default interval at which a traffic log outputs statistics.
const DefaultStatsInterval = 15 * time.Second

// MinimumStatsInterval is the minimum acceptable stats interval for traffic logs.
const MinimumStatsInterval = 500 * time.Millisecond

// CaptureStats holds information about packet capture statistics.
type CaptureStats struct {
	// Received is the total number of packets successfully processed.
	Received uint64

	// Dropped is the total number of packets dropped.
	Dropped uint64
}

func (cs CaptureStats) String() string {
	return fmt.Sprintf("received: %d; dropped: %d", cs.Received, cs.Dropped)
}

// statsTracker aggregates statistics from multiple channels.
type statsTracker struct {
	// Contrary to the CaptureStats documentation, the input channel receives statistics with counts
	// of *newly* received and dropped packets. That is, the total packets received and dropped is
	// the sum of the values received on the input channel.
	input     chan CaptureStats
	output    chan CaptureStats
	done      chan struct{}
	doneGroup sync.WaitGroup
}

func newStatsTracker(outputInterval time.Duration) *statsTracker {
	st := statsTracker{
		make(chan CaptureStats),
		make(chan CaptureStats, channelBufferSize),
		make(chan struct{}),
		sync.WaitGroup{},
	}
	st.doneGroup.Add(1)
	go func() {
		var (
			received, dropped uint64
			outputTimer       = time.NewTimer(outputInterval)
		)
		defer st.doneGroup.Done()
		defer outputTimer.Stop()
		for {
			select {
			case statsIn := <-st.input:
				received += statsIn.Received
				dropped += statsIn.Dropped
			case <-outputTimer.C:
				select {
				case st.output <- CaptureStats{received, dropped}:
				default:
				}
				outputTimer.Reset(outputInterval)
			case <-st.done:
				select {
				case st.output <- CaptureStats{received, dropped}:
				default:
				}
				return
			}
		}
	}()
	return &st
}

func (st *statsTracker) track(c <-chan CaptureStats) {
	st.doneGroup.Add(1)
	defer st.doneGroup.Done()

	var received, dropped uint64 // totals for this channel
	for {
		select {
		case stats, ok := <-c:
			if !ok {
				return
			}
			st.input <- CaptureStats{stats.Received - received, stats.Dropped - dropped}
			received, dropped = stats.Received, stats.Dropped
		case <-st.done:
			return
		}
	}
}

func (st *statsTracker) close() {
	close(st.done)
	st.doneGroup.Wait()
	close(st.input)
	close(st.output)
}

// Options for running a traffic log.
type Options struct {
	// MTULimit is the largest acceptable MTU size for interfaces watched by a traffic log. This can
	// be important in preventing the capture and save buffers from becoming far larger than
	// expected, which would have a significant impact on memory pressure.
	//
	// Defaults to DefaultMaxMTU.
	MTULimit int

	// A MutatorFactory is used to govern mutations which are made to packets upon capture. The
	// functions produced by this factory need not be concurrency-safe themselves, but separate
	// mutators may be called concurrently. These mutators must be capabale of keeping up with
	// packet ingress or packets will be dropped.
	//
	// Defaults to NoOpFactory.
	MutatorFactory MutatorFactory

	// StatsInterval is the interval at which the traffic log outputs statistics.
	//
	// Defaults to DefaultStatsInterval
	StatsInterval time.Duration
}

func (opts Options) mtuLimit() int {
	if opts.MTULimit == 0 {
		return DefaultMaxMTU
	}
	return opts.MTULimit
}

func (opts Options) mutatorFactory() MutatorFactory {
	if opts.MutatorFactory == nil {
		return new(NoOpFactory)
	}
	return opts.MutatorFactory
}

func (opts Options) statsInterval() time.Duration {
	if opts.StatsInterval <= 0 {
		return DefaultStatsInterval
	}
	if opts.StatsInterval < MinimumStatsInterval {
		return MinimumStatsInterval
	}
	return opts.StatsInterval
}

// TrafficLog is a log of network traffic.
//
// Captured packets are saved in a ring buffer and may be overwritten by newly captured packets. At
// any time, a group of packets can be saved by calling SaveCaptures. These saved captures can then
// be written out in pcapng format using WritePcapng.
type TrafficLog struct {
	captureBuffer    *sharedRingBuffer
	saveBuffer       *ringBuffer
	capturePool      *bpool.BufferPool
	savePool         *bpool.BufferPool
	captureProcs     map[string]*captureProcess
	captureProcsLock sync.Mutex
	statsTracker     *statsTracker
	errorChan        chan error
	mtuLimit         int
	mutatorFactory   MutatorFactory
	statsInterval    time.Duration
}

// New returns a new TrafficLog. Start capture by calling UpdateAddresses. The options may be nil in
// which case the defaults will be used.
//
// Captured packets will be saved in a fixed-size ring buffer, the size of which is specified by
// captureBytes. At any time, a group of captured packets can be saved by calling SaveCaptures. This
// moves packets captured for a specified address and time period into a separate fixed-size ring
// buffer. The size of this buffer is specified by saveBytes.
//
// In choosing the size of the packet buffers, note that the traffic log itself has an overhead of
// about 660 KB. Per-packet overhead is already accounted for in maintaining the buffers.
func New(captureBytes, saveBytes int, opts *Options) *TrafficLog {
	if opts == nil {
		opts = &Options{}
	}
	return &TrafficLog{
		newSharedRingBuffer(captureBytes),
		newRingBuffer(saveBytes),
		bpool.NewBufferPool(dataPoolSize),
		bpool.NewBufferPool(dataPoolSize),
		map[string]*captureProcess{},
		sync.Mutex{},
		newStatsTracker(opts.statsInterval()),
		make(chan error, channelBufferSize),
		opts.mtuLimit(),
		opts.mutatorFactory(),
		opts.statsInterval(),
	}
}

// UpdateAddresses updates the addresses for which traffic is being captured. Capture will begin (or
// continue) for all addresses in the input slice. Capture will be stopped for any addresses not in
// the input slice.
//
// If the network interface used to reach the address has an MTU exceeding MaxAcceptableMTU, this
// will result in an error.
//
// If an error is returned, the addresses have not been updated. In other words, a partial update is
// not possible.
func (tl *TrafficLog) UpdateAddresses(addresses []string) error {
	tl.captureProcsLock.Lock()
	defer tl.captureProcsLock.Unlock()

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
			proc, err := startCapture(
				addr, tl.captureBuffer.newHook(), tl.capturePool,
				tl.mutatorFactory, tl.mtuLimit, tl.statsInterval/procStatsPerLogStats)
			if err != nil {
				stopAllNewCaptures()
				return errors.New("failed to start capture for %s: %v", addr, err)
			}
			captureProcs[addr] = proc
			go tl.statsTracker.track(proc.statsChan)
			go tl.watchErrors(proc.errorChan)
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

// UpdateBufferSizes imposes new limits on the size of the capture and save buffers. These buffers
// are not immediately resized - the update will take effect as new packets arrive.
func (tl *TrafficLog) UpdateBufferSizes(captureBytes, saveBytes int) {
	tl.captureBuffer.updateCap(captureBytes)
	tl.saveBuffer.updateCap(saveBytes)
}

// SaveCaptures saves all captures for the given address received in the past duration d. These
// captured packets will be copied from the main capture buffer into a fixed-size ring buffer
// specifically for saved captures. Saved packets will only be overwritten upon future calls to
// SaveCaptures.
func (tl *TrafficLog) SaveCaptures(address string, d time.Duration) {
	tl.captureProcsLock.Lock()
	proc, ok := tl.captureProcs[address]
	tl.captureProcsLock.Unlock()
	if !ok {
		// Not really an error as it's possible the capture process has simply stopped.
		return
	}

	sinceNano := time.Now().Add(-1 * d).UnixNano()
	proc.forEach(func(pkt capturedPacket) {
		if pkt.info.unixNano > sinceNano {
			// Note: writes to bytes.Buffers do not return errors.
			newBuf := tl.savePool.Get()
			newBuf.Write(pkt.dataBuf.Bytes())
			pkt.dataBuf = newBuf
			pkt.dataPool = tl.savePool
			tl.saveBuffer.put(pkt)
		}
	})
}

// WritePcapng writes saved captures in pcapng file format.
func (tl *TrafficLog) WritePcapng(w io.Writer) error {
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
			LinkType:    iface.linkType.gopacketLinkType(),
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
	tl.saveBuffer.forEach(func(item bufferItem) {
		pkt := item.(capturedPacket)
		id, err := registerInterface(pkt.info.iface)
		if err != nil {
			numErrors++
			lastError = errors.New("failed to register interface: %v", err)
			return
		}
		gopacketCI := pkt.info.gopacketCI()
		// Oddly, the pcapgo package expects this to be the registration ID.
		gopacketCI.InterfaceIndex = id
		if err := pcapW.WritePacket(gopacketCI, pkt.dataBuf.Bytes()); err != nil {
			numErrors++
			lastError = errors.New("failed to write packet: %v", err)
			return
		}
	})
	if err := pcapW.Flush(); err != nil {
		return errors.New("failed to flush writer: %v", err)
	}
	if numErrors > 0 {
		return errors.New("%d errors writing packets; last error: %v", numErrors, lastError)
	}
	return nil
}

// Stats returns a channel on which the traffic log will periodically output capture statistics.
// This channel is buffered and unread statistics will be dropped as needed. This channel will
// close if tl.Close is called.
func (tl *TrafficLog) Stats() <-chan CaptureStats {
	return tl.statsTracker.output
}

// Errors returns a channel on which the traffic log will output any errors which occur during
// packet capture. This channel is buffered and unread errors will be dropped as needed. This
// channel will close if tl.Close is called.
func (tl *TrafficLog) Errors() <-chan error {
	return tl.errorChan
}

// Close the TrafficLog. All captures will stop and the log will be cleared. Calling methods on a
// closed TrafficLog may cause a panic.
func (tl *TrafficLog) Close() error {
	tl.captureProcsLock.Lock()
	defer tl.captureProcsLock.Unlock()

	for _, proc := range tl.captureProcs {
		proc.stop()
	}
	tl.captureBuffer = nil
	tl.saveBuffer = nil
	tl.captureProcs = nil
	tl.statsTracker.close()
	close(tl.errorChan)
	return nil
}

func (tl *TrafficLog) watchErrors(errChan <-chan error) {
	for err := range errChan {
		select {
		case tl.errorChan <- err:
		default:
		}
	}
}
