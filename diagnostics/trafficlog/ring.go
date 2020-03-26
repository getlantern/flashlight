package trafficlog

import (
	"sync"
)

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

func (q *queue) forEach(f func(interface{})) {
	current := q.first
	for current != nil {
		f(current.value)
		current = current.next
	}
}

func (q *queue) empty() bool {
	return q.first == nil
}

type bufferItem interface {
	// size must return a consistent value or bugs (possibly panics) will result.
	size() int

	// onEvict is called when, during a put, the buffer needs to evict items to make room. For
	// performance reasons, this is invoked synchronously during the put. Care should be taken to
	// ensure this function is cheap.
	onEvict()
}

type ringBuffer struct {
	size, cap int
	q         queue

	sync.Mutex
}

func newRingBuffer(cap int) *ringBuffer {
	return &ringBuffer{0, cap, queue{}, sync.Mutex{}}
}

// put an item. As a special case, if the item size exceeds the buffer capacity, the buffer will be
// cleared out and the new item will be the only item in the buffer.
func (buf *ringBuffer) put(item bufferItem) {
	buf.Lock()
	defer buf.Unlock()

	itemSize := item.size()
	if itemSize > buf.cap {
		for !buf.q.empty() {
			buf.q.dequeue().(bufferItem).onEvict()
		}
		buf.q.enqueue(item)
		buf.size = itemSize
		return
	}

	for buf.size+itemSize > buf.cap {
		evicted := buf.q.dequeue().(bufferItem)
		buf.size -= evicted.size()
		evicted.onEvict()
	}
	buf.q.enqueue(item)
	buf.size += item.size()
}

// forEach applies the input function to each element currently in the buffer. The function is
// applied to elements in order of insertion. All other operations on this buffer will be blocked
// while forEach is running.
func (buf *ringBuffer) forEach(do func(bufferItem)) {
	buf.Lock()
	defer buf.Unlock()

	buf.q.forEach(func(i interface{}) {
		do(i.(bufferItem))
	})
}

// This does not have an immediate effect on the size of the buffer. The buffer will grow or shrink
// appropriately on the next put.
func (buf *ringBuffer) updateCap(cap int) {
	buf.Lock()
	buf.cap = cap
	buf.Unlock()
}

type sharedBufferHook struct {
	// put an item. As a special case, if the item size exceeds the buffer capacity, the buffer will
	// be cleared out and the new item will be the only item in the buffer.
	put func(bufferItem)

	// forEach applies a function to each existing item entered into the buffer using this hook.
	// Items are provided to the function in insertion order. This call blocks all calls on this and
	// other hooks into the buffer.
	forEach func(do func(bufferItem))

	// close the hook, signaling that it will no longer be used.
	close func()
}

type sharedRingBuffer struct {
	size, cap int

	// masterQueue is a queue of *queues. Entries point to queues tied to hooks. When an item is
	// put into the ring using a hook, a pointer to that hook's queue is added to masterQueue. Thus,
	// if the first entry in masterQueue points to someQueue, then someQueue holds the oldest item
	// in the ring.
	masterQueue *queue

	sync.Mutex
}

func newSharedRingBuffer(cap int) *sharedRingBuffer {
	return &sharedRingBuffer{0, cap, new(queue), sync.Mutex{}}
}

// This does not have an immediate effect on the size of the buffer. The buffer will grow or shrink
// appropriately on the next put.
func (buf *sharedRingBuffer) updateCap(cap int) {
	buf.Lock()
	buf.cap = cap
	buf.Unlock()
}

func (buf *sharedRingBuffer) newHook() *sharedBufferHook {
	buf.Lock()
	defer buf.Unlock()

	q := new(queue)
	closed := false

	return &sharedBufferHook{
		put: func(item bufferItem) {
			buf.Lock()
			defer buf.Unlock()

			if closed {
				return
			}

			itemSize := item.size()
			if itemSize > buf.cap {
				for !buf.masterQueue.empty() {
					evictingFrom := buf.masterQueue.dequeue().(*queue)
					evictingFrom.dequeue().(bufferItem).onEvict()
				}
				q.enqueue(item)
				buf.masterQueue.enqueue(q)
				buf.size = itemSize
				return
			}

			for buf.size+itemSize > buf.cap {
				// Note: calling the eviction function in a new goroutine would avoid the
				// possibility of blocking the put function. However, the overhead of spawning new
				// goroutines proved too much to keep up with packet ingress.
				evictingFrom := buf.masterQueue.dequeue().(*queue)
				evicted := evictingFrom.dequeue().(bufferItem)
				buf.size -= evicted.size()
				evicted.onEvict()
			}
			q.enqueue(item)
			buf.masterQueue.enqueue(q)
			buf.size += itemSize
		},

		forEach: func(do func(bufferItem)) {
			buf.Lock()
			defer buf.Unlock()

			q.forEach(func(i interface{}) {
				do(i.(bufferItem))
			})
		},

		close: func() {
			buf.Lock()
			closed = true
			buf.Unlock()
		},
	}
}
