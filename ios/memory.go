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
)

// Memory management on iOS is critical because we're running in a network extension that's limited to 15 MB of memory. We handle this using several techniques.
//
// 1. We force garbage collection frequently and immediately request that memory be released to the OS
// 2. We use the MemChecker abstraction to find out when we're running critically low on memory. If we are, we started forcibly closing the oldest connections
//    until we're no longer critically low. If that didn't work, we reset the IP stack completely.
//    - we close the most oldest connections first because those are likeliest to be idle
//    - This does disrupt the user experience for some connections, but it gives the tunnel a chance to continue working for others. The alternative is for the tunnel to
//      get terminated by the OS, which is more disruptive
//    - We check for memory pressure before accepting new connections
//

const (
	logMemoryInterval     = 5 * time.Second
	forcedGCInterval      = 1 * time.Second
	checkCriticalInterval = 15 * time.Millisecond
	postResetDelay        = 10 * time.Second
)

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
		// this select ensures that if ticker fired while freeMemory() was running (i.e. it took longer than forcedGCInterval), we wait until the ticket fires again to run freeMemory
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
	for c.memChecker.Check().Critical {
		captureProfiles()
		c.reduceMemoryPressure()
	}
}

func (c *client) reduceMemoryPressure() {
	numConnsClosed, resolved := c.closeConnectionsToReduceMemoryPressure()

	if resolved {
		return
	}

	statsLog.Debugf("Memory still critically low after closing %d connections, resetting client completely", numConnsClosed)
	c.clientWriter.Reset()

	// stop accepting connections for a bit to keep things from getting worse, and wait a bit for
	// connections to finish closing, then GC
	c.acceptMx.Lock()
	time.Sleep(postResetDelay)
	c.acceptMx.Unlock()

	freeMemory()
	c.logMemory()

	if c.memChecker.Check().Critical {
		// panic("Memory still critically low, panicking")
	}
}

func (c *client) closeConnectionsToReduceMemoryPressure() (numConnsClosed int, resolved bool) {
	// stop traffic while we try to free memory
	c.acceptMx.Lock()
	defer c.acceptMx.Unlock()

	for {
		if c.tcpHandler.closeOldestConn() || c.udpHandler.closeOldestConn() {
			statsLog.Debug("Memory critically low, closed 1 old connection")
			numConnsClosed++

			freeMemory()
			c.logMemory()

			if !c.memChecker.Check().Critical {
				statsLog.Debugf("Closing %d connections resolved memory pressure", numConnsClosed)
				resolved = true
				return
			}
		} else {
			// no more connections to close
			return
		}
	}
}

func (c *client) logMemory() {
	memstats := &runtime.MemStats{}
	runtime.ReadMemStats(memstats)
	memInfo := c.memChecker.Check()
	if memInfo != nil {
		statsLog.Debugf("Memory System: %v    Go InUse: %v    Go Alloc: %v    Go Sys: %v",
			humanize.Bytes(uint64(memInfo.Bytes)),
			humanize.Bytes(memstats.HeapInuse),
			humanize.Bytes(memstats.Alloc),
			humanize.Bytes(memstats.Sys))
	}
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
	path := getProfilePath()
	if path == "" {
		log.Error("No profile path set, can't capture profiles")
		return
	}

	heap, err := os.OpenFile(filepath.Join(path, "heap.profile"), os.O_TRUNC|os.O_CREATE|os.O_RDWR|os.O_SYNC, 0644)
	if err != nil {
		log.Errorf("Unable to open heap profile file %v for writing: %v", path, err)
		return
	}
	defer heap.Close()

	goroutine, err := os.OpenFile(filepath.Join(path, "goroutine_profile.txt"), os.O_TRUNC|os.O_CREATE|os.O_RDWR|os.O_SYNC, 0644)
	if err != nil {
		log.Errorf("Unable to open heap profile file %v for writing: %v", path, err)
		return
	}
	defer goroutine.Close()

	err = pprof.Lookup("heap").WriteTo(heap, 0)
	if err != nil {
		log.Errorf("Unable to capture heap profile: %v", err)
	} else {
		log.Debugf("Captured heap profile")
	}

	err = pprof.Lookup("goroutine").WriteTo(goroutine, 1)
	if err != nil {
		log.Errorf("Unable to capture goroutine profile: %v", err)
	} else {
		log.Debugf("Captured goroutine profile")
	}
}
