package diagnostics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSharedRingBuffer(t *testing.T) {
	t.Parallel()

	const (
		bufferSize   = 12
		numberHooks  = 4 // should be a factor of bufferSize
		itemsPerHook = bufferSize / numberHooks

		// The amount of time we wait for deletes to process.
		deleteWaitTime = 50 * time.Millisecond
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
	deleteChan := make(chan int, bufferSize)
	itemNumber := 0
	for i := 0; i < itemsPerHook; i++ {
		expectedItems[i] = i
	}
	for i := 0; i < numberHooks; i++ {
		for j := 0; j < len(expectedItems); j++ {
			num := itemNumber
			itemNumber++
			hooks[i].put(expectedItems[j], func() { deleteChan <- num })
		}
	}
	for i := 0; i < numberHooks; i++ {
		requireHookEquals(t, expectedItems, hooks[i])
	}

	select {
	case <-deleteChan:
		t.Fatal("unexpected invocation of delete callback")
	case <-time.After(deleteWaitTime):
		// Expected path
	}

	newHook := rb.newHook()
	for i := 0; i < itemsPerHook; i++ {
		num := itemNumber
		itemNumber++
		newHook.put(expectedItems[i], func() { deleteChan <- num })
		requireHookEquals(t, expectedItems[i+1:], hooks[0])
		select {
		case deleted := <-deleteChan:
			require.Equal(t, i, deleted)
		case <-time.After(deleteWaitTime):
			t.Fatal("timed out waiting for delete")
		}
	}
	requireHookEquals(t, expectedItems, newHook)

	hooks[0].put(99, nil)
	requireHookEquals(t, []int{99}, hooks[0])
	select {
	case deleted := <-deleteChan:
		require.Equal(t, itemsPerHook, deleted)
	case <-time.After(deleteWaitTime):
		t.Fatal("timed out waiting for delete")
	}

	hooks[0].close()
	hooks[0].put(99, nil)
	// A closed hook should not force existing entries out.
	requireHookEquals(t, expectedItems[1:], hooks[1])
	select {
	case <-deleteChan:
		t.Fatal("unexpected invocation of delete callback")
	case <-time.After(deleteWaitTime):
		// Expected path
	}
}
