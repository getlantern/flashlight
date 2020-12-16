package ios

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/getlantern/flashlight/chained"
)

// Memory management on iOS is critical because we're running in a network extension that's limited to 15 MB of memory. We handle this using several techniques.
//
// 1. Set an aggressive GCPercent
// 2. Use the MemChecker abstraction to find out when we're running critically low on memory. If we are, we start forcibly closing the oldest connections in batches
//    until we're no longer critically low. If that didn't work, we reset our TCP and UDP handlers as well as the balancer.
//    - We close the oldest connections first because those are likeliest to be idle
//    - We close multiple connections at once because single connections rarely relieve enough memory pressure
//    - This does disrupt the user experience for some connections, but it gives the tunnel a chance to continue working for others. The alternative is for the tunnel to
//      get terminated by the OS, which is more disruptive
// 3. While resolving low memory conditions, we do not accept new connections or upload packets for existing TCP connections
// 4. Dialing to the proxy with TLS connections is quite memory intensive due to the public key cryptography, so we limit the number of concurrent dials
// 5. Use short idle timeouts to reduce the number of simultaneously open connections
//

const (
	logMemoryInterval                = 5 * time.Second
	forceGCInterval                  = 25 * time.Millisecond
	checkCriticalInterval            = 25 * time.Millisecond
	postResetDelay                   = 10 * time.Second
	postFreeDelay                    = 50 * time.Millisecond
	cyclesToWaitForMemoryReduction   = 4
	numberOfConnectionsToCloseAtOnce = 4
)

func init() {
	// set more aggressive IdleTimeout to help deal with memory constraints on iOS
	chained.IdleTimeout = 15 * time.Second

	// set an aggressive target for triggering GC after new allocations reach 20% of heap
	debug.SetGCPercent(20)
}

var (
	profilePath   string
	profilePathMx sync.RWMutex
)

func SetProfilePath(path string) {
	profilePathMx.Lock()
	defer profilePathMx.Unlock()
	profilePath = path
}

func getProfilePath() string {
	profilePathMx.Lock()
	defer profilePathMx.Unlock()
	return profilePath
}

// MemChecker checks the system's memory level
type MemChecker interface {
	// BytesBeforeCritical returns the number of bytes of memory left available for use before getting critically low
	BytesBeforeCritical() int
}

func (c *client) trackMemory() {
	for {
		c.logMemory()
		time.Sleep(logMemoryInterval)
	}
}

func (c *client) gcPeriodically() {
	ticker := time.NewTicker(forceGCInterval)
	for range ticker.C {
		// this select ensures that if ticker fired while we were checking memory (i.e. it took longer than forceGCInterval), we wait until the ticket fires again to check memory
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
		// this select ensures that if ticker fired while we were checking memory (i.e. it took longer than checkCriticalInterval), we wait until the ticket fires again to check memory
		select {
		case <-ticker.C:
			continue
		default:
			c.reduceMemoryPressureIfNecessary()
		}
	}
}

func (c *client) reduceMemoryPressureIfNecessary() {
	if c.checkCriticalAndLogMemoryIfNecessary() {
		captureProfiles()
		c.reduceMemoryPressure()
	}

	if c.checkCriticalAndLogMemoryIfNecessary() {
		statsLog.Debug("Memory still critically low after taking all possible measures to reduce")
	}
}

func (c *client) reduceMemoryPressure() {
	numConnsClosed, resolved := c.closeOldestConnectionsToReduceMemoryPressure()

	if resolved {
		return
	}

	statsLog.Debugf("Memory still critically low after closing %d connections, resetting client completely", numConnsClosed)
	c.clientWriter.Reset()

	// wwait a bit for connections to finish closing, then GC
	time.Sleep(postResetDelay)

	c.checkCriticalAndLogMemoryIfNecessary()
}

func (c *client) closeOldestConnectionsToReduceMemoryPressure() (totalNumConnsClosed int, resolved bool) {
	for {
		statsLog.Debugf("Memory still critically low, closing up to %d connections at once", numberOfConnectionsToCloseAtOnce)

		numClosed := 0
		for i := 0; i < numberOfConnectionsToCloseAtOnce; i++ {
			if !c.tcpHandler.closeOldestConn() && !c.udpHandler.closeOldestConn() {
				// nothing left to close
				break
			}
			numClosed++
		}

		if numClosed > 0 {
			statsLog.Debugf("Closed %d oldest connections", numClosed)
			totalNumConnsClosed += numClosed

			if !c.checkCriticalAndLogMemoryIfNecessary() {
				statsLog.Debugf("Closing a total of %d connections resolved memory pressure", totalNumConnsClosed)
				resolved = true
				return
			}
		} else {
			resolved = false // noop, just for clarity
			return
		}
	}
}

func (c *client) checkCriticalAndLogMemoryIfNecessary() bool {
	bytesBeforeCritical := 0
	for i := 0; i < cyclesToWaitForMemoryReduction; i++ {
		bytesBeforeCritical = c.memChecker.BytesBeforeCritical()
		c.memcap.setAvailable(bytesBeforeCritical)
		critical := bytesBeforeCritical < 0
		if !critical {
			// all good
			return false
		}

		// memory is critically low
		freeMemory()
		time.Sleep(postFreeDelay)
	}

	c.doLogMemory(bytesBeforeCritical)
	return true
}

func (c *client) logMemory() {
	bytesBeforeCritical := c.memChecker.BytesBeforeCritical()
	c.memcap.setAvailable(bytesBeforeCritical)
	c.doLogMemory(bytesBeforeCritical)
}

func (c *client) doLogMemory(bytesBeforeCritical int) {
	memstats := &runtime.MemStats{}
	runtime.ReadMemStats(memstats)

	if bytesBeforeCritical < 0 {
		bytesBeforeCritical = 0
	}

	statsLog.Debugf("Memory System Bytes before Critical: %v    Go InUse: %v    Go Alloc: %v",
		humanize.Bytes(uint64(bytesBeforeCritical)),
		humanize.Bytes(memstats.HeapInuse),
		humanize.Bytes(memstats.Alloc))

	stats := debug.GCStats{
		PauseQuantiles: make([]time.Duration, 10),
	}
	debug.ReadGCStats(&stats)
	elapsed := time.Now().Sub(c.started)
	statsLog.Debugf("Memory GC num: %v    total pauses: %v (%.2f%%)    pause percentiles: %v", stats.NumGC, stats.PauseTotal, float64(stats.PauseTotal)*100/float64(elapsed), stats.PauseQuantiles)
	statsLog.Debugf("Resets: %d", atomic.LoadInt64(&c.resets))
}

func freeMemory() {
	debug.FreeOSMemory() // this calls garbage collection before freeing memory to the OS
}

func captureProfiles() {
	log.Debug("Capturing profiles")

	// always free memory before capturing profiles because we need at least one GC before capturing heap data to get appropriate stats
	freeMemory()

	path := getProfilePath()
	if path == "" {
		log.Error("No profile path set, can't capture profiles")
		return
	}

	heap, err := os.OpenFile(filepath.Join(path, "heap.profile.tmp"), os.O_TRUNC|os.O_CREATE|os.O_RDWR|os.O_SYNC, 0644)
	if err != nil {
		log.Errorf("Unable to open heap profile file %v for writing: %v", path, err)
		return
	}
	defer heap.Close()

	goroutine, err := os.OpenFile(filepath.Join(path, "goroutine_profile.txt.tmp"), os.O_TRUNC|os.O_CREATE|os.O_RDWR|os.O_SYNC, 0644)
	if err != nil {
		log.Errorf("Unable to open heap profile file %v for writing: %v", path, err)
		return
	}
	defer goroutine.Close()

	err = pprof.WriteHeapProfile(heap)
	if err != nil {
		log.Errorf("Unable to capture heap profile: %v", err)
	} else {
		err = os.Rename(filepath.Join(path, "heap.profile.tmp"), filepath.Join(path, "heap.profile"))
		if err != nil {
			log.Errorf("Unable to rename heap profile: %v", err)
		} else {
			log.Debugf("Captured heap profile")
		}
	}

	err = pprof.Lookup("goroutine").WriteTo(goroutine, 1)
	if err != nil {
		log.Errorf("Unable to capture goroutine profile: %v", err)
	} else {
		err = os.Rename(filepath.Join(path, "goroutine_profile.txt.tmp"), filepath.Join(path, "goroutine_profile.txt"))
		if err != nil {
			log.Errorf("Unable to rename goroutine profile: %v", err)
		} else {
			log.Debugf("Captured goroutine profile")
		}
	}
}
