// Package safechannels provides abstractions for channels that can be safely closed even with pending writes.
package safechannels

import "sync"

// Bytes is a channel that can be used to write and read byte slices and that can be safely closed even with pending writes.
type Bytes interface {
	// Write writes to the channel. Returns if the write happened, false if it was aborted because the channel was closed.
	Write([]byte) bool

	// Read returns a channel from which one can read.
	Read() <-chan []byte

	// Close closes this channel. Can be called multiple times.
	Close()
}

type bytesChannel struct {
	mx        sync.RWMutex
	ch        chan []byte
	closedCh  chan interface{}
	closeOnce sync.Once
}

func NewBytes(size int) Bytes {
	return &bytesChannel{
		ch:       make(chan []byte, size),
		closedCh: make(chan interface{}),
	}
}

func (ch *bytesChannel) Write(data []byte) bool {
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

func (ch *bytesChannel) Read() <-chan []byte {
	return ch.ch
}

func (ch *bytesChannel) Close() {
	ch.closeOnce.Do(func() {
		// close closeCh first without protection, this is safe because no one ever writes to closedCh
		close(ch.closedCh)
		ch.mx.Lock()
		// close ch with protection to make sure we don't close it while someone is trying to write to it
		close(ch.ch)
		ch.mx.Unlock()
	})
}
