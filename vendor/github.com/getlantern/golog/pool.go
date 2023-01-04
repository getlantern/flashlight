package golog

import (
	"bytes"

	"github.com/oxtoacart/bpool"
)

const (
	// Together, these bound the buffer pool to 307 KB
	bufferPoolSize = 200
	maxBufferSize  = 1024
)

var (
	_bufferPool = bpool.NewBufferPool(bufferPoolSize)
)

func getBuffer() *bytes.Buffer {
	return _bufferPool.Get()
}

func returnBuffer(buf *bytes.Buffer) bool {
	if buf.Cap() <= maxBufferSize {
		_bufferPool.Put(buf)
		return true
	}
	return false
}
