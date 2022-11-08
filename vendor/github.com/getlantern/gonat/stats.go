package gonat

import (
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/getlantern/ops"
)

// StatsTracker tracks statistics for one or more gonat servers.
type StatsTracker struct {
	acceptedPackets   int64
	invalidPackets    int64
	droppedPackets    int64
	numServers        int64
	numServersClosing int64
	numServersClosed  int64
	numTCPConns       int64
	numUDPConns       int64
	numTCPConnsClosed int64
	numUDPConnsClosed int64
	statsInterval     time.Duration
	startOnce         sync.Once
	stop              chan interface{}
	stopped           chan interface{}
}

// NewStatsTracker creates a new StatsTracker that will log stats at the given statsInterval.
// Logging only begins once a Server using this StatsTracker is started, and continues until
// Stop is called
func NewStatsTracker(statsInterval time.Duration) *StatsTracker {
	return &StatsTracker{
		statsInterval: statsInterval,
		stop:          make(chan interface{}),
		stopped:       make(chan interface{}),
	}
}

// Stop stops the StatsTracker
func (s *StatsTracker) Close() error {
	select {
	case <-s.stop:
		// already stopped
	default:
		close(s.stop)
	}
	<-s.stopped
	return nil
}

func (s *StatsTracker) start() {
	s.startOnce.Do(func() {
		ops.Go(s.trackStats)
	})
}

func (s *StatsTracker) trackStats() {
	defer close(s.stopped)
	ticker := time.NewTicker(s.statsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			log.Debugf("Servers: %d    Closing: %d    Closed: %d", s.NumServers(), s.NumServersClosing(), s.NumServersClosed())
			log.Debugf("TCP Conns: %d    Closed: %d", s.NumTCPConns(), s.NumTCPConnsClosed())
			log.Debugf("UDP Conns: %d    Closed: %d", s.NumUDPConns(), s.NumUDPConnsClosed())
			log.Debugf("Invalid Packets: %d    Accepted Packets: %d    Dropped Packets: %d", s.InvalidPackets(), s.AcceptedPackets(), s.DroppedPackets())
		}
	}
}

func (s *StatsTracker) acceptedPacket() {
	atomic.AddInt64(&s.acceptedPackets, 1)
}

// AcceptedPackets gives a count of accepted packets
func (s *StatsTracker) AcceptedPackets() int {
	return int(atomic.LoadInt64(&s.acceptedPackets))
}

func (s *StatsTracker) invalidPacket() {
	atomic.AddInt64(&s.invalidPackets, 1)
}

// InvalidPackets gives a count of invalid packets (unknown destination, wrong IP version, etc.)
func (s *StatsTracker) InvalidPackets() int {
	return int(atomic.LoadInt64(&s.invalidPackets))
}

func (s *StatsTracker) droppedPacket() {
	atomic.AddInt64(&s.droppedPackets, 1)
}

// DroppedPackets gives a count of packets dropped due to being stalled writing down or upstream,
// being unable to assign a port open a connection, etc.
func (s *StatsTracker) DroppedPackets() int {
	return int(atomic.LoadInt64(&s.droppedPackets))
}

func (s *StatsTracker) serverStarted() {
	atomic.AddInt64(&s.numServers, 1)
}

func (s *StatsTracker) startClosingServer() {
	atomic.AddInt64(&s.numServersClosing, 1)
}

func (s *StatsTracker) serverClosed() {
	atomic.AddInt64(&s.numServersClosing, -1)
	atomic.AddInt64(&s.numServers, -1)
}

// NumServers gives a count of the number of gonat servers currently running
func (s *StatsTracker) NumServers() int {
	return int(atomic.LoadInt64(&s.numServers))
}

// NumServersClosing gives a count of the number of gonat servers currently closing
func (s *StatsTracker) NumServersClosing() int {
	return int(atomic.LoadInt64(&s.numServersClosing))
}

// NumServersClosed gives a count of the number of gonat servers closed
func (s *StatsTracker) NumServersClosed() int {
	return int(atomic.LoadInt64(&s.numServersClosed))
}

func (s *StatsTracker) openedConn(proto uint8) {
	switch proto {
	case syscall.IPPROTO_TCP:
		atomic.AddInt64(&s.numTCPConns, 1)
	case syscall.IPPROTO_UDP:
		atomic.AddInt64(&s.numUDPConns, 1)
	}
}

func (s *StatsTracker) closedConn(proto uint8) {
	switch proto {
	case syscall.IPPROTO_TCP:
		atomic.AddInt64(&s.numTCPConnsClosed, 1)
		atomic.AddInt64(&s.numTCPConns, -1)
	case syscall.IPPROTO_UDP:
		atomic.AddInt64(&s.numUDPConnsClosed, 1)
		atomic.AddInt64(&s.numUDPConns, -1)
	}
}

// NumTCPConns gives a count of the number of TCP connections being tracked
func (s *StatsTracker) NumTCPConns() int {
	return int(atomic.LoadInt64(&s.numTCPConns))
}

// NumUDPConns gives a count of the number of UDP connections being tracked
func (s *StatsTracker) NumUDPConns() int {
	return int(atomic.LoadInt64(&s.numUDPConns))
}

// NumTCPConnsClosed gives a count of the number of TCP connections that have been closed
func (s *StatsTracker) NumTCPConnsClosed() int {
	return int(atomic.LoadInt64(&s.numTCPConnsClosed))
}

// NumUDPConnsClosed gives a count of the number of UDP connections that have been closed
func (s *StatsTracker) NumUDPConnsClosed() int {
	return int(atomic.LoadInt64(&s.numUDPConnsClosed))
}
