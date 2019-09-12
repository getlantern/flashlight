package diagnostics

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestByteSliceRing(t *testing.T) {
	// Write 10 slices of length 10 to a ring of length 100.

	buf := newByteSliceRingBuffer(100)
	for i := 0; i < 100; i = i + 10 {
		b := []byte{}
		for j := 0; j < 10; j++ {
			b = append(b, byte(i+j))
		}
		n, err := buf.Write(b)
		require.Equal(t, 10, n)
		require.NoError(t, err)
	}

	bytesBuf := new(bytes.Buffer)
	n, err := buf.WriteTo(bytesBuf)
	require.NoError(t, err)
	require.Equal(t, int64(100), n)

	bufBytes := bytesBuf.Bytes()
	require.Equal(t, 100, len(bufBytes))

	for i := 0; i < 100; i++ {
		require.Equal(t, i, int(bufBytes[i]))
	}

	// Write 10 slices of length 10 to a ring of length 99. The first slice should get deleted to
	// make room for the last.

	buf = newByteSliceRingBuffer(99)
	for i := 0; i < 100; i = i + 10 {
		b := []byte{}
		for j := 0; j < 10; j++ {
			b = append(b, byte(i+j))
		}
		n, err := buf.Write(b)
		require.Equal(t, 10, n)
		require.NoError(t, err)
	}

	bytesBuf = new(bytes.Buffer)
	n, err = buf.WriteTo(bytesBuf)
	require.NoError(t, err)
	require.Equal(t, int64(90), n)

	bufBytes = bytesBuf.Bytes()
	require.Equal(t, 90, len(bufBytes))

	for i := 0; i < 90; i++ {
		require.Equal(t, i+10, int(bufBytes[i]))
	}

	// Write 2 slices of length 10 to a ring of length 19. Again, the first slice should get deleted.

	buf = newByteSliceRingBuffer(19)
	for i := 0; i < 20; i = i + 10 {
		b := []byte{}
		for j := 0; j < 10; j++ {
			b = append(b, byte(i+j))
		}
		n, err := buf.Write(b)
		require.Equal(t, 10, n)
		require.NoError(t, err)
	}

	bytesBuf = new(bytes.Buffer)
	n, err = buf.WriteTo(bytesBuf)
	require.NoError(t, err)
	require.Equal(t, int64(10), n)

	bufBytes = bytesBuf.Bytes()
	require.Equal(t, 10, len(bufBytes))

	for i := 0; i < 10; i++ {
		require.Equal(t, i+10, int(bufBytes[i]))
	}
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
