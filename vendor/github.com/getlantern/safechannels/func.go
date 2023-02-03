package safechannels

import "sync"

// Func is a channel that can be used to write and read no-argument functions and that can be safely closed even with pending writes.
type Func interface {
	// Write writes to the channel. Returns if the write happened, false if it was aborted because the channel was closed.
	Write(func()) bool

	// Read returns a channel from which one can read.
	Read() <-chan func()

	// Close closes this channel. Can be called multiple times.
	Close()
}

type funcChannel struct {
	mx        sync.RWMutex
	ch        chan func()
	closedCh  chan interface{}
	closeOnce sync.Once
}

func NewFunc(size int) Func {
	return &funcChannel{
		ch:       make(chan func(), size),
		closedCh: make(chan interface{}),
	}
}

func (ch *funcChannel) Write(data func()) bool {
	ch.mx.RLock()
	defer ch.mx.RUnlock()

	select {
	case <-ch.closedCh:
		// already closed before we tried writing
		return false
	default:
		select {
		case ch.ch <- data:
			// successfully wrote
			return true
		case <-ch.closedCh:
			// closed while waiting for channel to become available for write
			return false
		}
	}
}

func (ch *funcChannel) Read() <-chan func() {
	return ch.ch
}

func (ch *funcChannel) Close() {
	ch.closeOnce.Do(func() {
		// close closeCh first without protection, this is safe because no one ever writes to closedCh
		close(ch.closedCh)
		ch.mx.Lock()
		// close ch with protection to make sure we don't close it while someone is trying to write to it
		close(ch.ch)
		ch.mx.Unlock()
	})
}
