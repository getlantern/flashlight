package buffers

const (
	maxBufferBytes = 128 * 1024 // use a small amount of buffers on iOS to avoid running afoul of memory limit
)
