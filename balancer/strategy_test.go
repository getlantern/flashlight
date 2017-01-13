package balancer

import (
	"container/heap"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStickyStrategy(t *testing.T) {
	d1 := &dialer{consecSuccesses: 3, consecFailures: 0}
	d2 := &dialer{consecSuccesses: 4, consecFailures: 0}

	h := Sticky([]*dialer{d1, d2})
	heap.Init(&h)
	assert.Equal(t, heap.Pop(&h), d2, "should select dialer with more successes")
}

func TestFastestStrategy(t *testing.T) {
	d1 := &dialer{estimatedThroughput: 99}
	d2 := &dialer{estimatedThroughput: 100}
	d3 := &dialer{estimatedThroughput: 0}

	h := Fastest([]*dialer{d1, d2, d3})
	heap.Init(&h)
	assert.Equal(t, heap.Pop(&h), d2, "should select faster dialer with non-zero throughput")
}

func TestQualityFirstStrategy(t *testing.T) {
	d1 := &dialer{consecSuccesses: 3, consecFailures: 0, estimatedThroughput: 100}
	d2 := &dialer{consecSuccesses: 4, consecFailures: 0, estimatedThroughput: 10}
	d3 := &dialer{consecSuccesses: 0, consecFailures: 1, estimatedThroughput: 100}

	h := QualityFirst([]*dialer{d1, d2})
	heap.Init(&h)
	assert.Equal(t, heap.Pop(&h), d1, "should select faster dialer when both has positive successes")

	h = QualityFirst([]*dialer{d2, d3})
	heap.Init(&h)
	assert.Equal(t, heap.Pop(&h), d2, "should select more reliable dialer even if it's slower")
}
