package chained

import (
	"sync"
	"time"
)

// rater provides a mechanism for accumulating a count over time and tracking
// the rate at which that count is changing. We remember the min and max of this
// rate over time.
//
// Time begins with a call to begin().
//
// Time advances with a call to advance().
//
// The rater is recalculated with each call to calc().
//
// The final values can be obtained atomically using get().
type rater struct {
	start            time.Time
	end              time.Time
	total            int
	snapshottedTotal int
	lastSnapshotted  time.Time
	min              float64
	max              float64
	mx               sync.Mutex
}

// begin sets the start time for calculating rates.
func (r *rater) begin(ts func() time.Time) {
	r.mx.Lock()
	if r.start.IsZero() {
		r.start = ts()
	}
	r.mx.Unlock()
}

// advance adds n to the internal count as of ts.
func (r *rater) advance(n int, ts time.Time) {
	r.mx.Lock()
	r.total += n
	r.end = ts
	r.mx.Unlock()
}

// calc recalculates the internal EMA rate and updates the min/max accordingly.
func (r *rater) calc() {
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

	if !hasSnapshotted || newRate < r.min {
		r.min = newRate
	}
	if !hasSnapshotted || newRate > r.max {
		r.max = newRate
	}
	r.snapshottedTotal = r.total
	r.lastSnapshotted = r.end

	r.mx.Unlock()
}

// get atomically returns the total count and the min, max and average rates
// over the duration of this rater.
func (r *rater) get() (total float64, min float64, max float64, average float64) {
	r.mx.Lock()
	defer r.mx.Unlock()
	total = float64(r.total)
	min = r.min
	max = r.max
	deltaSeconds := r.end.Sub(r.start).Seconds()
	if deltaSeconds > 0 {
		average = float64(r.total) / deltaSeconds
	}
	return
}
