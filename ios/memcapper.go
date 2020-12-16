package ios

import (
	"sync"
	"sync/atomic"
	"time"
)

type memCapper struct {
	cond      *sync.Cond
	available int
	stalled   int64
}

func newMemCapper(initialAvailable int) *memCapper {
	var mx sync.Mutex
	mc := &memCapper{
		cond:      sync.NewCond(&mx),
		available: initialAvailable,
	}
	go mc.trackStalledOnly()
	go mc.trackAll()
	return mc
}

func (mc *memCapper) allowed(n int) bool {
	mc.cond.L.Lock()
	defer mc.cond.L.Unlock()

	if mc.available > n {
		mc.available -= n
		return true
	}

	return false
	// 	atomic.AddInt64(&mc.stalled, 1)
	// 	mc.cond.Wait()
	// 	atomic.AddInt64(&mc.stalled, -1)
	// }
}

func (mc *memCapper) setAvailable(available int) {
	mc.cond.L.Lock()
	mc.available = available
	mc.cond.Broadcast()
	mc.cond.L.Unlock()
}

func (mc *memCapper) trackStalledOnly() {
	for {
		time.Sleep(250 * time.Millisecond)
		stalled := atomic.LoadInt64(&mc.stalled)
		if stalled > 0 {
			statsLog.Debugf("%d currently stalled", stalled)
		}
	}
}

func (mc *memCapper) trackAll() {
	for {
		time.Sleep(5 * time.Second)
		stalled := atomic.LoadInt64(&mc.stalled)
		statsLog.Debugf("%d currently stalled", stalled)
	}
}
