package safechannels

import "sync"

type IOResult struct {
	N   int
	Err error
}

// IO is a channel that can be used to write and read io results and that can be safely closed even with pending writes.
type IO interface {
	// Write writes to the channel. Returns if the write happened, false if it was aborted because the channel was closed.
	Write(int, error) bool

	// Read returns a channel from which one can read.
	Read() <-chan *IOResult

	// Close closes this channel. Can be called multiple times.
	Close()
}

type ioChannel struct {
	mx        sync.RWMutex
	ch        chan *IOResult
	closedCh  chan interface{}
	closeOnce sync.Once
}

func NewIO(size int) IO {
	return &ioChannel{
		ch:       make(chan *IOResult, size),
		closedCh: make(chan interface{}),
	}
}

func (ch *ioChannel) Write(n int, err error) bool {
	data := &IOResult{n, err}

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

func (ch *ioChannel) Read() <-chan *IOResult {
	return ch.ch
}

func (ch *ioChannel) Close() {
	ch.closeOnce.Do(func() {
		// close closeCh first without protection, this is safe because no one ever writes to closedCh
		close(ch.closedCh)
		ch.mx.Lock()
		// close ch with protection to make sure we don't close it while someone is trying to write to it
		close(ch.ch)
		ch.mx.Unlock()
	})
}
