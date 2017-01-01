package balancer

import (
	"sync"
	"time"
)

type rate struct {
	start            time.Time
	end              time.Time
	total            int
	snapshottedTotal int
	lastSnapshotted  time.Time
	min              float64
	max              float64
	avg              *ema
	mx               sync.Mutex
}

func (r *rate) begin(ts func() time.Time) {
	r.mx.Lock()
	if r.start.IsZero() {
		r.start = ts()
	}
	r.mx.Unlock()
}

func (r *rate) update(n int, ts time.Time) {
	r.mx.Lock()
	r.total += n
	r.end = ts
	r.mx.Unlock()
}

func (r *rate) snapshot() {
	r.mx.Lock()
	if r.start.IsZero() || r.end.IsZero() {
		// Not yet started or nothing recorded, can't snapshot yet
		r.mx.Unlock()
		return
	}

	hasSnapshotted := !r.lastSnapshotted.IsZero()
	if !hasSnapshotted {
		r.lastSnapshotted = r.start
	}

	deltaSeconds := r.end.Sub(r.lastSnapshotted).Seconds()
	if deltaSeconds == 0 {
		// Not enough time elapsed, can't snapshot
		r.mx.Unlock()
		return
	}
	delta := float64(r.total - r.snapshottedTotal)
	newRate := delta / deltaSeconds

	var avg float64
	if r.avg == nil {
		r.avg = newEMA(newRate, 0.5)
		avg = newRate
	} else {
		avg = r.avg.update(newRate)
	}

	if !hasSnapshotted || avg < r.min {
		r.min = avg
	}
	if !hasSnapshotted || avg > r.max {
		r.max = avg
	}
	r.snapshottedTotal = r.total
	r.lastSnapshotted = r.end

	r.mx.Unlock()
}

func (r *rate) average() float64 {
	deltaSeconds := r.end.Sub(r.start).Seconds()
	if deltaSeconds == 0 {
		return 0
	}
	return float64(r.total) / deltaSeconds
}
