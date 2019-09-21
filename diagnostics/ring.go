package diagnostics

import (
	"sync"
)

type ringBuffer struct {
	len, maxLen int
	q           queue

	sync.Mutex
}

func newRingBuffer(maxLen int) *ringBuffer {
	return &ringBuffer{0, maxLen, queue{}, sync.Mutex{}}
}

func (buf *ringBuffer) put(i interface{}) {
	buf.Lock()
	defer buf.Unlock()

	if buf.len+1 > buf.maxLen {
		buf.q.dequeue()
		buf.len--
	}
	buf.q.enqueue(i)
	buf.len++
}

// forEach applies the input function to each element currently in the buffer. The function is
// applied to elements in order of insertion. All other operations on this buffer will be blocked
// while forEach is running.
func (buf *ringBuffer) forEach(do func(interface{})) {
	buf.Lock()
	defer buf.Unlock()

	buf.q.forEach(do)
}

type sharedBufferHook struct {
	// put an item into the buffer. onDelete may be nil.
	put func(item interface{}, onDelete func())

	// forEach applies a function to each existing item entered into the buffer using this hook.
	// Items are provided to the function in insertion order. This call blocks all calls on this and
	// other hooks into the buffer.
	forEach func(do func(interface{}))

	// close the hook, signaling that it will no longer be used.
	close func()
}

type sharedRingBufferItem struct {
	value    interface{}
	onDelete func()
}

type sharedRingBuffer struct {
	len, maxLen int

	// queues in the map correspond to outstanding hooks. Each is assigned an identifying integer.
	queues map[int]*queue

	// masterQueue is a queue of integers representing the order in which items were added to the
	// ring. For example, if the oldest entry in masterQueue is 2, then queue 2 holds the oldest
	// item in the ring.
	masterQueue *queue

	sync.Mutex
}

func newSharedRingBuffer(maxLen int) *sharedRingBuffer {
	return &sharedRingBuffer{0, maxLen, map[int]*queue{}, new(queue), sync.Mutex{}}
}

func (buf *sharedRingBuffer) newHook() *sharedBufferHook {
	buf.Lock()
	defer buf.Unlock()

	q := new(queue)
	qIndex := len(buf.queues)
	buf.queues[qIndex] = q
	closed := false

	return &sharedBufferHook{
		put: func(i interface{}, onDelete func()) {
			buf.Lock()
			defer buf.Unlock()

			if closed {
				return
			}

			if onDelete == nil {
				onDelete = func() {}
			}

			for buf.len+1 > buf.maxLen {
				dequeueNumber := buf.masterQueue.dequeue().(int)
				dequeueingFrom, ok := buf.queues[dequeueNumber]
				if !ok {
					// This queue was closed, try the next one.
					continue
				}
				dequeued := dequeueingFrom.dequeue()
				go dequeued.(sharedRingBufferItem).onDelete()
				buf.len = buf.len - 1
			}
			q.enqueue(sharedRingBufferItem{i, onDelete})
			buf.masterQueue.enqueue(qIndex)
			buf.len++
		},

		forEach: func(do func(interface{})) {
			buf.Lock()
			defer buf.Unlock()

			q.forEach(func(i interface{}) {
				do(i.(sharedRingBufferItem).value)
			})
		},

		close: func() {
			buf.Lock()
			defer buf.Unlock()

			delete(buf.queues, qIndex)
			closed = true
		},
	}
}
