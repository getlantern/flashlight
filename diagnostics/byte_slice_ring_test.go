package diagnostics

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestByteSliceRing(t *testing.T) {
	// Write 10 slices of length 10 to a ring of length 100.

	buf := newByteSliceRingMap(100)
	expectedValues := map[string][]byte{}
	for i := 0; i < 100; i = i + 10 {
		b := []byte{}
		for j := 0; j < 10; j++ {
			b = append(b, byte(i+j))
		}
		key := strconv.Itoa(i)
		require.NoError(t, buf.put(key, b, func() { t.Fatal("unexpected deletion") }))
		expectedValues[key] = b
	}

	buf.forEach(func(key string, value []byte) {
		expectedValue, ok := expectedValues[key]
		require.True(t, ok, "unexpected key: %s", key)
		require.Equal(t, expectedValue, value)
	})

	// Write 10 slices of length 10 to a ring of length 99. The first slice should get deleted to
	// make room for the last.

	buf = newByteSliceRingMap(99)
	expectedValues = map[string][]byte{}
	onDeleteCalled := false
	for i := 0; i < 100; i = i + 10 {
		b := []byte{}
		for j := 0; j < 10; j++ {
			b = append(b, byte(i+j))
		}
		key := strconv.Itoa(i)
		if i == 0 {
			require.NoError(t, buf.put(key, b, func() { onDeleteCalled = true }))
		} else {
			require.NoError(t, buf.put(key, b, nil))
			expectedValues[key] = b
		}
	}

	require.True(t, onDeleteCalled)
	buf.forEach(func(key string, value []byte) {
		expectedValue, ok := expectedValues[key]
		require.True(t, ok, "unexpected key: %s", key)
		require.Equal(t, expectedValue, value)
	})

	// Write 2 slices of length 10 to a ring of length 19. Again, the first slice should get deleted.

	buf = newByteSliceRingMap(19)
	expectedValues = map[string][]byte{}
	onDeleteCalled = false
	for i := 0; i < 20; i = i + 10 {
		b := []byte{}
		for j := 0; j < 10; j++ {
			b = append(b, byte(i+j))
		}
		key := strconv.Itoa(i)
		if i == 0 {
			require.NoError(t, buf.put(key, b, func() { onDeleteCalled = true }))
		} else {
			require.NoError(t, buf.put(key, b, nil))
			expectedValues[key] = b
		}
	}

	require.True(t, onDeleteCalled)
	buf.forEach(func(key string, value []byte) {
		expectedValue, ok := expectedValues[key]
		require.True(t, ok, "unexpected key: %s", key)
		require.Equal(t, expectedValue, value)
	})
}

func TestByteSliceQueue(t *testing.T) {
	q := new(byteSliceQueue)
	for i := 0; i < 10; i++ {
		q.enqueue([]byte{byte(i)})
	}
	for i := 0; i < 10; i++ {
		require.Equal(t, i, int(q.dequeue()[0]))
	}

	q = new(byteSliceQueue)
	for i := 0; i < 10; i++ {
		q.enqueue([]byte{byte(i)})
	}
	for i := 0; i < 5; i++ {
		q.dequeue()
	}
	for i := 10; i < 20; i++ {
		q.enqueue([]byte{byte(i)})
	}
	for i := 5; i < 20; i++ {
		require.Equal(t, i, int(q.dequeue()[0]))
	}

	// Further dequeue calls should just return nil.
	for i := 0; i < 3; i++ {
		require.NotPanics(t, func() { require.Nil(t, q.dequeue()) })
	}

	// The queue should still be usable.
	for i := 0; i < 10; i++ {
		q.enqueue([]byte{byte(i)})
	}
	for i := 0; i < 10; i++ {
		require.Equal(t, i, int(q.dequeue()[0]))
	}
}
