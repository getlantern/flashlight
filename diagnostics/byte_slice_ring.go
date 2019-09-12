package diagnostics

import (
	"io"

	"github.com/getlantern/errors"
)

type ringMapEntry struct {
	key      string
	value    []byte
	onDelete func()
}

// byteSliceRingMap acts as both a ring buffer and a map. The structure is initialized with a fixed
// maximum size. As slices are written, old slices are deleted (in FIFO order) as necessary to make
// room. Slices are written along with a key which can then be used to query the slice, provided it
// has not yet been deleted.
// TODO: this needs to be concurrency safe
type byteSliceRingMap struct {
	q                *queue
	m                map[string][]byte
	totalLen, maxLen int
}

func newRingMap(maxLen int) *byteSliceRingMap {
	return &byteSliceRingMap{new(queue), map[string][]byte{}, 0, maxLen}
}

// put a new slice in the ring map. This may cause old slices to get deleted. onDelete may be nil.
func (buf *byteSliceRingMap) put(b []byte, key string, onDelete func()) error {
	if onDelete == nil {
		onDelete = func() {}
	}
	if len(b) > buf.maxLen {
		return errors.New("slice (len %d) is larger than ring (len %d)", len(b), buf.maxLen)
	}

	for buf.totalLen+len(b) > buf.maxLen {
		dequeued := buf.q.dequeue().(ringMapEntry)
		buf.totalLen = buf.totalLen - len(dequeued.value)
		delete(buf.m, dequeued.key)
		dequeued.onDelete()
	}
	buf.q.enqueue(ringMapEntry{key, b, onDelete})
	buf.m[key] = b
	buf.totalLen = buf.totalLen + len(b)
	return nil
}

func (buf *byteSliceRingMap) get(key string) (b []byte, ok bool) {
	b, ok = buf.m[key]
	return
}

func (buf *byteSliceRingMap) forEach(do func(key string, value []byte)) {
	for k, v := range buf.m {
		do(k, v)
	}
}

// TODO: evaluate whether this needs to be concurrency-safe
type byteSliceRing struct {
	slices           *byteSliceQueue
	totalLen, maxLen int
}

func newByteSliceRingBuffer(maxLen int) *byteSliceRing {
	return &byteSliceRing{new(byteSliceQueue), 0, maxLen}
}

// Write the slice to the buffer. If the slice overflows the buffer, the oldest slice(s) will be
// deleted to make room. Slices are deleted in their entirety.
func (buf *byteSliceRing) Write(b []byte) (n int, err error) {
	if len(b) > buf.maxLen {
		return 0, errors.New("slice (len %d) is larger than ring (len %d)", len(b), buf.maxLen)
	}

	for buf.totalLen+len(b) > buf.maxLen {
		buf.totalLen = buf.totalLen - len(buf.slices.dequeue())
	}
	buf.slices.enqueue(b)
	buf.totalLen = buf.totalLen + len(b)
	return len(b), nil
}

// WriteTo implements the io.WriterTo interface, writing the contents of the ring to w.
func (buf *byteSliceRing) WriteTo(w io.Writer) (n int64, err error) {
	var currentN int
	buf.slices.forEach(func(b []byte) {
		if err != nil {
			// The io.WriterTo interfaces specifies that we stop on the first error.
			return
		}
		currentN, err = w.Write(b)
		n = n + int64(currentN)
	})
	return
}

type queueNode struct {
	next  *queueNode
	value interface{}
}

// A queue with FIFO semantics. The zero value is an empty, ready-to-use queue.
type queue struct {
	first, last *queueNode
}

func (q *queue) enqueue(i interface{}) {
	if q.first == nil {
		q.first = &queueNode{nil, i}
		q.last = q.first
		return
	}
	prevLast := q.last
	q.last = &queueNode{nil, i}
	prevLast.next = q.last
}

// Returns nil if the queue is empty.
func (q *queue) dequeue() interface{} {
	if q.first == nil {
		return nil
	}
	dequeued := q.first
	q.first = dequeued.next
	if q.first == nil {
		q.last = nil
	}
	return dequeued.value
}

type byteSliceQueueNode struct {
	next  *byteSliceQueueNode
	value []byte
}

// A queue with FIFO semantics. The zero value is an empty, ready-to-use queue.
type byteSliceQueue struct {
	first, last *byteSliceQueueNode
}

func (q *byteSliceQueue) enqueue(b []byte) {
	if q.first == nil {
		q.first = &byteSliceQueueNode{nil, b}
		q.last = q.first
		return
	}
	prevLast := q.last
	q.last = &byteSliceQueueNode{nil, b}
	prevLast.next = q.last
}

// Returns nil if the queue is empty.
func (q *byteSliceQueue) dequeue() []byte {
	if q.first == nil {
		return nil
	}
	dequeued := q.first
	q.first = dequeued.next
	if q.first == nil {
		q.last = nil
	}
	return dequeued.value
}

func (q *byteSliceQueue) forEach(f func(b []byte)) {
	current := q.first
	for current != nil {
		f(current.value)
		current = current.next
	}
}
