package chained

import (
	"testing"
	"time"

	"github.com/getlantern/mtime"
	"github.com/stretchr/testify/assert"
)

func TestRater(t *testing.T) {
	r := &rater{}
	r.calc()

	// No activity yet
	total, min, max, average := r.get()
	assert.EqualValues(t, 0, total)
	assert.EqualValues(t, 0, min)
	assert.EqualValues(t, 0, max)
	assert.EqualValues(t, 0, average)

	ts := mtime.Now()
	r.begin(func() mtime.Instant {
		return ts
	})
	total, min, max, average = r.get()
	assert.EqualValues(t, 0, total)
	assert.EqualValues(t, 0, min)
	assert.EqualValues(t, 0, max)
	assert.EqualValues(t, 0, average)

	ts = ts.Add(1 * time.Second)
	// begin should have no effect after first call
	r.begin(func() mtime.Instant {
		return ts
	})
	r.advance(1, ts)
	total, min, max, average = r.get()
	assert.EqualValues(t, 1, total)
	assert.EqualValues(t, 0, min)
	assert.EqualValues(t, 0, max)
	assert.EqualValues(t, 1, average)

	r.calc()
	total, min, max, average = r.get()
	assert.EqualValues(t, 1, total)
	assert.EqualValues(t, 1, min)
	assert.EqualValues(t, 1, max)
	assert.EqualValues(t, 1, average)

	ts = ts.Add(2 * time.Second)
	// begin should have no effect after first call
	r.begin(func() mtime.Instant {
		return ts
	})
	r.advance(4, ts)
	r.calc()
	total, min, max, average = r.get()
	assert.EqualValues(t, 5, total)
	assert.EqualValues(t, 1, min)
	assert.EqualValues(t, 2, max)
	assert.EqualValues(t, 5.0/3.0, average)

	// Simulate period of inactivity
	ts = ts.Add(3 * time.Second)
	r.advance(0, ts)
	r.calc()
	total, min, max, average = r.get()
	assert.EqualValues(t, 5, total)
	assert.EqualValues(t, 0, min)
	assert.EqualValues(t, 2, max)
	assert.EqualValues(t, 5.0/6.0, average)
}
