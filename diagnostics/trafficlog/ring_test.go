package trafficlog

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/oxtoacart/bpool"
	"github.com/stretchr/testify/require"
)

type testItem struct {
	value    int
	itemSize int
	evicted  *bool
}

func newTestItem(value, size int) *testItem {
	evicted := false
	return &testItem{value, size, &evicted}
}

func (ti *testItem) size() int {
	return ti.itemSize
}

func (ti *testItem) onEvict() {
	*ti.evicted = true
}

func (ti *testItem) equals(other *testItem) bool {
	// We only compare on value and size.
	return ti.value == other.value && ti.size() == other.size()
}

func (ti *testItem) String() string {
	return fmt.Sprintf("{value: %v, size: %d}", ti.value, ti.size())
}

func TestSharedRingBufferPutAndEviction(t *testing.T) {
	t.Parallel()

	const (
		bufferSize   = 12
		numberHooks  = 4 // should be a factor of bufferSize
		itemsPerHook = bufferSize / numberHooks
	)

	rb := newSharedRingBuffer(bufferSize)
	hooks := []*sharedBufferHook{}
	for i := 0; i < numberHooks; i++ {
		hooks = append(hooks, rb.newHook())
	}

	// The initial batch of items will all share an eviction tracker.
	putItems := make([]*testItem, bufferSize)
	anyEvicted := false
	for i := 0; i < bufferSize; i++ {
		putItems[i] = &testItem{i, 1, &anyEvicted}
	}

	// Test that hooks only see their own items and in the correct order.
	for i := 0; i < numberHooks; i++ {
		for j := 0; j < itemsPerHook; j++ {
			hooks[i].put(putItems[itemsPerHook*i+j])
		}
	}
	for i := 0; i < numberHooks; i++ {
		start := itemsPerHook * i
		requireHookEquals(t, putItems[start:start+itemsPerHook], hooks[i])
	}
	require.False(t, anyEvicted)

	// Now we give each item its own eviction tracker.
	for _, item := range putItems {
		evicted := false
		item.evicted = &evicted
	}

	// Test eviction order and puts into a full buffer.
	newHook := rb.newHook()
	for i := 0; i < itemsPerHook; i++ {
		newItem := newTestItem(i, 1)
		newHook.put(newItem)
		putItems = append(putItems, newItem)
		requireHookEquals(t, putItems[i+1:itemsPerHook], hooks[0])
		require.True(t, *putItems[i].evicted)
	}
	requireHookEquals(t, putItems[bufferSize:], newHook)

	// Test puts into a hook post-eviction.
	newItem := newTestItem(99, 1)
	hooks[0].put(newTestItem(99, 1))
	requireHookEquals(t, []*testItem{newItem}, hooks[0])
	require.True(t, *putItems[itemsPerHook].evicted)

	// Test puts into a closed hook.
	hooks[0].close()
	hooks[0].put(newTestItem(99, 1))
	requireHookEquals(t, putItems[itemsPerHook+1:2*itemsPerHook], hooks[1])
	require.False(t, *putItems[itemsPerHook+1].evicted)
}

func TestSharedRingBufferItemSizing(t *testing.T) {
	t.Parallel()

	rb := newSharedRingBuffer(3)
	h1, h2, h3 := rb.newHook(), rb.newHook(), rb.newHook()
	i1, i2, i3 := []*testItem{}, []*testItem{}, []*testItem{}
	i1 = append(i1, newTestItem(1, 1), newTestItem(1, 1))
	h1.put(i1[0])
	h1.put(i1[1])
	requireHookEquals(t, i1, h1)
	i2 = append(i2, newTestItem(2, 2))
	h2.put(i2[0])
	requireHookEquals(t, i1[1:], h1)
	requireHookEquals(t, i2, h2)
	i3 = append(i3, newTestItem(3, 2))
	h3.put(i3[0])
	requireHookEquals(t, []*testItem{}, h1)
	requireHookEquals(t, []*testItem{}, h2)
	requireHookEquals(t, i3, h3)
	i1 = append(i1, newTestItem(1, 1))
	h1.put(i1[2])
	requireHookEquals(t, i1[2:], h1)
	requireHookEquals(t, []*testItem{}, h2)
	requireHookEquals(t, i3, h3)
}

func TestSharedRingBufferResizing(t *testing.T) {
	t.Parallel()

	rb := newSharedRingBuffer(10)
	h := rb.newHook()

	items := make([]*testItem, rb.cap)
	for i := 0; i < rb.cap; i++ {
		items[i] = newTestItem(i, 1)
		h.put(items[i])
	}

	rb.updateCap(5)
	newItem := newTestItem(10, 1)
	items = append(items, newItem)
	h.put(newItem)
	for i := 0; i < 6; i++ {
		require.True(t, *items[i].evicted)
	}
	requireHookEquals(t, items[6:], h)

	rb.updateCap(10)
	newItems := make([]*testItem, 10)
	for i := 100; i < 110; i++ {
		newItems[i-100] = newTestItem(i, 1)
		h.put(newItems[i-100])
	}
	for _, item := range items {
		require.True(t, *item.evicted)
	}
	requireHookEquals(t, newItems, h)
}

func requireHookEquals(t *testing.T, expected []*testItem, h *sharedBufferHook) {
	t.Helper()

	hookItems := []*testItem{}
	h.forEach(func(item bufferItem) {
		hookItems = append(hookItems, item.(*testItem))
	})
	if len(expected) != len(hookItems) {
		require.FailNow(t, "hook does not have expected items",
			"expected: %s\nhook: %s", sPrintItems(expected), sPrintItems(hookItems))
	}
	for i := 0; i < len(expected); i++ {
		if !expected[i].equals(hookItems[i]) {
			require.FailNow(t, "hook does not have expected items",
				"expected: %s\nhook: %s", sPrintItems(expected), sPrintItems(hookItems))
		}
	}
}

func sPrintItems(items []*testItem) string {
	itemStrings := make([]string, len(items))
	for i, item := range items {
		itemStrings[i] = item.String()
	}
	return fmt.Sprint(itemStrings)
}

type benchmarkItem struct {
	evictFunc func()
}

func (bi benchmarkItem) size() int { return 1 }
func (bi benchmarkItem) onEvict()  { bi.evictFunc() }

func makeItemsForBenchmark(n int) []bufferItem {
	// Eviction callbacks for the sharedRingBuffer are used to put buffers back into a buffer pool.
	// Barring edge cases with which we are not concerned (extremely high packet ingress rates),
	// this pool is never full.
	bp := bpool.NewBufferPool(n + 1)
	items := make([]bufferItem, n)
	for i := 0; i < n; i++ {
		buf := new(bytes.Buffer)
		items[i] = benchmarkItem{func() { bp.Put(buf) }}
	}
	return items
}

// BenchmarkSharedRingBuffer measures the performance of a full ring buffer (the steady state). This
// performance is important in keeping up with packet ingress.
func BenchmarkSharedRingBuffer(b *testing.B) {
	const bufferSize = 100 // shouldn't matter much

	hook := newSharedRingBuffer(bufferSize).newHook()
	for _, item := range makeItemsForBenchmark(bufferSize) {
		hook.put(item)
	}
	newItems := makeItemsForBenchmark(b.N)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		hook.put(newItems[i])
	}
}
