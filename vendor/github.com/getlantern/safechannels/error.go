package safechannels

import "sync"

// Error is a channel that can be used to write and read errors and that can be safely closed even with pending writes.
type Error interface {
	// Write writes to the channel. Returns if the write happened, false if it was aborted because the channel was closed.
	Write(error) bool

	// Read returns a channel from which one can read.
	Read() <-chan error

	// Close closes this channel. Can be called multiple times.
	Close()
}

type errorChannel struct {
	mx        sync.RWMutex
	ch        chan error
	closedCh  chan interface{}
	closeOnce sync.Once
}

func NewError(size int) Error {
	return &errorChannel{
		ch:       make(chan error, size),
		closedCh: make(chan interface{}),
	}
}

func (ch *errorChannel) Write(data error) bool {
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

func (ch *errorChannel) Read() <-chan error {
	return ch.ch
}

func (ch *errorChannel) Close() {
	ch.closeOnce.Do(func() {
		// close closeCh first without protection, this is safe because no one ever writes to closedCh
		close(ch.closedCh)
		ch.mx.Lock()
		// close ch with protection to make sure we don't close it while someone is trying to write to it
		close(ch.ch)
		ch.mx.Unlock()
	})
}
