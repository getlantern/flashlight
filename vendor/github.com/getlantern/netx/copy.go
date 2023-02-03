package netx

import (
	"io"
	"net"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/getlantern/errors"
)

var (
	copyTimeout = 1 * time.Second
)

// CopyOpts provides options for BidiCopy. It will use sensible defaults for any missing options
type CopyOpts struct {
	BufIn          []byte
	BufOut         []byte
	OnOut          func(int)
	OnIn           func(int)
	StartGoroutine func(func())
}

func (opts *CopyOpts) ApplyDefaults() {
	if opts.BufIn == nil {
		opts.BufIn = make([]byte, 65536)
	}
	if opts.BufOut == nil {
		opts.BufOut = make([]byte, 65536)
	}
	if opts.OnOut == nil {
		opts.OnOut = func(int) {}
	}
	if opts.OnIn == nil {
		opts.OnIn = func(int) {}
	}
	if opts.StartGoroutine == nil {
		opts.StartGoroutine = basicStartGoroutine
	}
}

// BidiCopy copies between in and out in both directions using the specified
// buffers, returning the errors from copying to out and copying to in.
func BidiCopy(out net.Conn, in net.Conn, bufOut []byte, bufIn []byte) (outErr error, inErr error) {
	outErrCh, inErrCh := BidiCopyWithOpts(out, in, &CopyOpts{
		BufIn:  bufIn,
		BufOut: bufOut,
	})
	return <-outErrCh, <-inErrCh
}

// BidiCopyWithOpts is like the original BidiCopy but providing more options and returning channels for reading the errors rather than the errors themselves.
func BidiCopyWithOpts(out net.Conn, in net.Conn, opts *CopyOpts) (outErr <-chan error, inErr <-chan error) {
	opts.ApplyDefaults()
	stop := uint32(0)
	outErrCh := make(chan error, 1)
	inErrCh := make(chan error, 1)
	go doCopy(out, in, opts.BufIn, outErrCh, &stop, opts.OnOut, opts.StartGoroutine)
	go doCopy(in, out, opts.BufOut, inErrCh, &stop, opts.OnIn, opts.StartGoroutine)
	return outErrCh, inErrCh
}

// doCopy is based on io.copyBuffer
func doCopy(dst net.Conn, src net.Conn, buf []byte, errCh chan error, stop *uint32, cb func(int), startGoroutine func(func())) {
	var err error
	defer func() {
		atomic.StoreUint32(stop, 1)
		dst.SetReadDeadline(time.Now().Add(copyTimeout))
		errCh <- err
		close(errCh)
	}()

	defer func() {
		p := recover()
		if p != nil {
			err = errors.New("Panic while copying: %v\n%v", p, string(debug.Stack()))
		}
	}()

	for {
		stopping := atomic.LoadUint32(stop) == 1
		if stopping {
			src.SetReadDeadline(time.Now().Add(copyTimeout))
		}
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if ew != nil {
				err = ew
				return
			}
			if nw != nr {
				err = io.ErrShortWrite
				return
			}
			cb(nw)
		}
		if er == io.EOF {
			return
		}
		if er != nil {
			if IsTimeout(er) {
				if stopping {
					return
				}
			} else {
				err = er
				return
			}
		}
	}
}

// IsTimeout indicates whether the given error is a network timeout error
func IsTimeout(err error) bool {
	if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
		return true
	}
	return false
}

func basicStartGoroutine(fn func()) {
	go fn()
}
