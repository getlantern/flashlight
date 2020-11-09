package ios

import (
	"runtime"
	"runtime/debug"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/eycorsican/go-tun2socks/core"
)

// Memory management on iOS is critical because we're running in a network extension that's limited to 15 MB of memory. We handle this using several techniques.
//
// 1. We force garbage collection frequently and immediately request that memory be released to the OS
// 2. We use the MemChecker abstraction to find out when we're running critically low on memory. If we are, we started forcibly closing the most recent connections
//    until we're no longer critically low. If that didn't work, we reset the IP stack completely.
//    - we close the most recently used connections first because those are likeliest to be consuming a lot of memory
//    - This does disrupt the user experience for some connections, but it gives the tunnel a chance to continue working for others. The alternative is for the tunnel to
//      get terminated by the OS, which is more disruptive.
//

const (
	logMemoryInterval     = 5 * time.Second
	forcedGCInterval      = 1 * time.Second
	checkCriticalInterval = 15 * time.Millisecond
)

func init() {
	// Use a more aggressive idle timeout to reduce memory usage
	// chained.IdleTimeout = 15 * time.Second
}

// MemChecker checks the system's memory level
type MemChecker interface {
	// Check checks system memory
	Check() *MemInfo
}

// MemInfo provides information about system memory usage
type MemInfo struct {
	// Bytes gives the total memory in use
	Bytes int
	// Critical indicates if memory levels are getting critical
	Critical bool
}

func (c *client) trackMemory() {
	for {
		c.logMemory()
		time.Sleep(logMemoryInterval)
	}
}

func periodicGC() {
	debug.SetGCPercent(20)
	ticker := time.NewTicker(forcedGCInterval)
	for range ticker.C {
		select {
		case <-ticker.C:
			continue
		default:
			freeMemory()
		}
	}
}

func (c *client) checkForCriticallyLowMemory() {
	ticker := time.NewTicker(checkCriticalInterval)
	for range ticker.C {
		select {
		case <-ticker.C:
			continue
		default:
			for c.memChecker.Check().Critical {
				if c.tcpHandler.closeNewestConn() {
					statsLog.Debug("Memory critically low, closed newest TCP connection")
					freeMemory()
					c.logMemory()
				} else if c.udpHandler.closeNewestConn() {
					statsLog.Debug("Memory critically low, closed newest UDP connection")
					freeMemory()
					c.logMemory()
				} else {
					statsLog.Debug("Memory critically low, resetting client")
					c.clientWriter.Reset()
					freeMemory()
					c.logMemory()
				}
			}
		}
	}
}

func (h *proxiedTCPHandler) closeNewestConn() bool {
	downstream, ok := h.mruConns.removeNewest()
	if ok {
		downstream.Close()
	}
	return ok
}

// closes the oldest UDP connection in the LRU cache to help relieve memory pressure
func (h *directUDPHandler) closeNewestConn() bool {
	downstream, ok := h.mruConns.removeNewest()
	if ok {
		h.RLock()
		upstream := h.upstreams[downstream.(core.UDPConn)]
		h.RUnlock()
		upstream.Close() // we don't close downstream because that'll happen automatically once upstream finishes closing
	}
	return ok
}

func (c *client) logMemory() {
	memstats := &runtime.MemStats{}
	runtime.ReadMemStats(memstats)
	memInfo := c.memChecker.Check()
	statsLog.Debugf("Memory System: %v    Go InUse: %v    Go Alloc: %v    Go Sys: %v",
		humanize.Bytes(uint64(memInfo.Bytes)),
		humanize.Bytes(memstats.HeapInuse),
		humanize.Bytes(memstats.Alloc),
		humanize.Bytes(memstats.Sys))
	stats := debug.GCStats{
		PauseQuantiles: make([]time.Duration, 10),
	}
	debug.ReadGCStats(&stats)
	elapsed := time.Now().Sub(c.started)
	statsLog.Debugf("Memory GC num: %v    total pauses: %v (%.2f%%)    pause percentiles: %v", stats.NumGC, stats.PauseTotal, float64(stats.PauseTotal)*100/float64(elapsed), stats.PauseQuantiles)
}

func freeMemory() {
	debug.FreeOSMemory() // this calls garbage collection before freeing memory to the OS
}
