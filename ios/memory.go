package ios

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/getlantern/flashlight/chained"
)

// Memory management on iOS is critical because we're running in a network extension that's limited to 15 MB of memory. We handle this using several techniques.
//
// 1. Use a fork of go-tun2socks tuned for low memory usage (see https://lwip.fandom.com/wiki/Tuning_TCP and https://lwip.fandom.com/wiki/Lwipopts.h)
// 2. Set an aggressive GCPercent
// 3. Use the MemChecker abstraction to find out how much memory is left before hitting our limit and use this to limit the throughput of IP packets to/from tun2socks. As the system becomes memory constrained, we start dropping packets.
// 4. Dialing to the proxy with TLS connections is quite memory intensive due to the public key cryptography, so we limit the number of concurrent dials
// 5. Use short idle timeouts to reduce the number of simultaneously open connections

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
	// BytesRemain returns the number of bytes of memory left before we hit the system limit
	BytesRemain() int
}

func (c *client) trackMemory() {
	for {
		c.doTrackMemory()
		time.Sleep(trackMemoryInterval)
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

func (c *client) doTrackMemory() {
	bytesRemain := c.memChecker.BytesRemain()

	memstats := &runtime.MemStats{}
	runtime.ReadMemStats(memstats)

	if bytesRemain < 0 {
		bytesRemain = 0
	}

	statsLog.Debugf("Memory System Bytes Remain: %v    Go InUse: %v    Go Alloc: %v",
		humanize.Bytes(uint64(bytesRemain)),
		humanize.Bytes(memstats.HeapInuse),
		humanize.Bytes(memstats.Alloc))

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
