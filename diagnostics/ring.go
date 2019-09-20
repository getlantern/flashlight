package diagnostics

import (
	"sync"
)

type sharedBufferHook struct {
	// put an item into the buffer.
	put func(interface{})

	// get the oldest existing item entered into the buffer using this hook. Returns nil if no such
	// items exist in the buffer.
	// TODO: probably don't need this
	get func() interface{}

	// forEach applies a function to each existing item entered into the buffer using this hook.
	// Items are provided to the function in insertion order. This call blocks all calls on this and
	// other hooks into the buffer.
	forEach func(do func(interface{}))

	// close the hook, signaling that it will no longer be used.
	close func()
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
		put: func(i interface{}) {
			buf.Lock()
			defer buf.Unlock()

			if closed {
				return
			}

			for buf.len+1 > buf.maxLen {
				dequeueNumber := buf.masterQueue.dequeue().(int)
				dequeueingFrom, ok := buf.queues[dequeueNumber]
				if !ok {
					// This queue was closed, try the next one.
					continue
				}
				dequeueingFrom.dequeue()
				buf.len = buf.len - 1
			}
			q.enqueue(i)
			buf.masterQueue.enqueue(qIndex)
			buf.len++
		},

		get: func() interface{} {
			buf.Lock()
			defer buf.Unlock()

			return q.peek()
		},

		forEach: func(do func(interface{})) {
			buf.Lock()
			defer buf.Unlock()

			q.forEach(do)
		},

		close: func() {
			buf.Lock()
			defer buf.Unlock()

			delete(buf.queues, qIndex)
			closed = true
		},
	}
}
