// Package event provides support for event dispatching and listening.
package event

import (
	"sync/atomic"
)

// Dispatcher dispatches events.
type Dispatcher interface {
	// AddListener adds a new listener function for messages. The returned
	// function can and should be used to close the listener.
	AddListener(onMsg func(msg interface{})) (close func())

	// Dispatch dispatches a message to all registered listeners.
	Dispatch(msg interface{})

	// Close closes this dispatcher.
	Close()
}

type dispatcher struct {
	listenerIdx uint64
	blocking    bool
	bufferDepth int
	inCh        chan interface{}
}

// NewDispatcher creates a new Dispatcher. bufferDepth controls the depth of
// every listener's message queue, set to 0 to disable buffering completely.
// If blocking is true, every dispatch will wait for the message to be enqueued
// for all listeners. If blocking is false, messages to slow listeners whose
// buffers are full will be dropped.
func NewDispatcher(blocking bool, bufferDepth int) Dispatcher {
	d := &dispatcher{
		blocking:    blocking,
		bufferDepth: bufferDepth,
		inCh:        make(chan interface{}),
	}
	go d.dispatchLoop()

	return d
}

type listener struct {
	id    uint64
	onMsg func(msg interface{})
	msgCh chan interface{}
}

func (d *dispatcher) AddListener(onMsg func(msg interface{})) (close func()) {
	l := &listener{
		id:    atomic.AddUint64(&d.listenerIdx, 1),
		onMsg: onMsg,
		msgCh: make(chan interface{}, d.bufferDepth),
	}
	go l.acceptLoop()
	close = func() {
		d.inCh <- l.id
	}
	d.inCh <- l
	return close
}

func (d *dispatcher) Dispatch(msg interface{}) {
	d.inCh <- msg
}

func (d *dispatcher) Close() {
	close(d.inCh)
}

func (d *dispatcher) dispatchLoop() {
	listeners := make(map[uint64]*listener)
	for in := range d.inCh {
		switch t := in.(type) {
		case *listener:
			listeners[t.id] = t
		case uint64:
			close(listeners[t].msgCh)
			delete(listeners, t)
		default:
			for _, l := range listeners {
				if d.blocking {
					l.msgCh <- t
				} else {
					select {
					case l.msgCh <- t:
						// accepted
					default:
						// dropped
					}
				}
			}
		}
	}
}

func (l *listener) acceptLoop() {
	for msg := range l.msgCh {
		l.onMsg(msg)
	}
}
