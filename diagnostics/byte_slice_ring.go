package diagnostics

import (
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

func newByteSliceRingMap(maxLen int) *byteSliceRingMap {
	return &byteSliceRingMap{new(queue), map[string][]byte{}, 0, maxLen}
}

// put a new slice in the ring map. This may cause old slices to get deleted. onDelete may be nil.
func (buf *byteSliceRingMap) put(key string, b []byte, onDelete func()) error {
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
