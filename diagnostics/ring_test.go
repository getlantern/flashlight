package diagnostics

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTest(t *testing.T) {
	p, d := 83164, 8879
	fmt.Printf("%.1f\n", 100*float64(d)/(float64(p+d)))
}

func TestSharedRingBuffer(t *testing.T) {
	t.Parallel()

	const (
		bufferSize   = 12
		numberHooks  = 4 // should be a factor of bufferSize
		itemsPerHook = bufferSize / numberHooks
	)

	requireHookEquals := func(t *testing.T, expected []int, h *sharedBufferHook) {
		t.Helper()

		i := 0
		h.forEach(func(item interface{}) {
			require.Equal(t, expected[i], item)
			i++
		})
		require.Equal(t, len(expected), i, "less items in hook than expected")
	}

	rb := newSharedRingBuffer(bufferSize)
	hooks := []*sharedBufferHook{}
	for i := 0; i < numberHooks; i++ {
		hooks = append(hooks, rb.newHook())
	}

	expectedItems := make([]int, itemsPerHook)
	for i := 0; i < itemsPerHook; i++ {
		expectedItems[i] = i
	}
	for i := 0; i < numberHooks; i++ {
		for j := 0; j < len(expectedItems); j++ {
			hooks[i].put(expectedItems[j], nil)
		}
	}
	for i := 0; i < numberHooks; i++ {
		requireHookEquals(t, expectedItems, hooks[i])
	}

	newHook := rb.newHook()
	for i := 0; i < itemsPerHook; i++ {
		newHook.put(expectedItems[i], nil)
		requireHookEquals(t, expectedItems[i+1:], hooks[0])
	}
	requireHookEquals(t, expectedItems, newHook)

	hooks[0].put(99, nil)
	requireHookEquals(t, []int{99}, hooks[0])

	hooks[0].close()
	hooks[0].put(99, nil)
	// A closed hook should not force existing entries out.
	requireHookEquals(t, expectedItems[1:], hooks[1])

	// TODO: test delete callback
}
