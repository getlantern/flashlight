package balancer

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestRate(t *testing.T) {
	r := &rate{}
	r.snapshot()

	// No activity yet
	assert.EqualValues(t, 0, r.min)
	assert.EqualValues(t, 0, r.max)
	assert.EqualValues(t, 0, r.average())

	ts := time.Now()
	r.begin(func() time.Time {
		return ts
	})
	assert.EqualValues(t, 0, r.min)
	assert.EqualValues(t, 0, r.max)
	assert.EqualValues(t, 0, r.average())

	ts = ts.Add(1 * time.Second)
	// begin should have no effect after first call
	r.begin(func() time.Time {
		return ts
	})
	r.update(1, ts)
	assert.EqualValues(t, 0, r.min)
	assert.EqualValues(t, 0, r.max)
	assert.EqualValues(t, 1, r.average())

	r.snapshot()
	assert.EqualValues(t, 1, r.min)
	assert.EqualValues(t, 1, r.max)
	assert.EqualValues(t, 1, r.average())

	ts = ts.Add(2 * time.Second)
	// begin should have no effect after first call
	r.begin(func() time.Time {
		return ts
	})
	r.update(4, ts)
	r.snapshot()
	assert.EqualValues(t, 1, r.min)
	assert.EqualValues(t, 1.5, r.max)
	assert.EqualValues(t, 5.0/3.0, r.average())
}
