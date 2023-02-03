package psmux

import (
	"container/heap"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/big"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultAcceptBacklog = 1024
)

var (
	ErrInvalidProtocol = errors.New("invalid protocol")
	ErrConsumed        = errors.New("peer consumed more than sent")
	ErrGoAway          = errors.New("stream id overflows, should start a new connection")
	ErrTimeout         = &timeoutError{}
	ErrWouldBlock      = errors.New("operation would block on IO")
)

var _ net.Error = &timeoutError{}

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

type writeRequest struct {
	prio   uint64
	frame  Frame
	result chan writeResult
	flush  bool
}

type writeResult struct {
	n   int
	err error
}

// Session defines a multiplexed connection for streams
type Session struct {
	conn io.ReadWriteCloser

	config           *Config
	nextStreamID     uint32 // next stream identifier
	nextStreamIDLock sync.Mutex

	bucket       int32         // token bucket
	bucketNotify chan struct{} // used for waiting for tokens

	streams    map[uint32]*Stream // all streams in this session
	streamLock sync.Mutex         // locks streams

	die     chan struct{} // flag session has died
	dieOnce sync.Once

	// socket error handling
	socketReadError      atomic.Value
	socketWriteError     atomic.Value
	chSocketReadError    chan struct{}
	chSocketWriteError   chan struct{}
	socketReadErrorOnce  sync.Once
	socketWriteErrorOnce sync.Once

	// smux protocol errors
	protoError     atomic.Value
	chProtoError   chan struct{}
	protoErrorOnce sync.Once

	chAccepts chan *Stream

	dataReady int32 // flag data has arrived

	goAway int32 // flag id exhausted

	deadline atomic.Value

	shaper chan writeRequest // a shaper for writing
	writes chan writeRequest
}

func newSession(config *Config, conn io.ReadWriteCloser, client bool) *Session {
	s := new(Session)
	s.die = make(chan struct{})
	s.conn = conn
	s.config = config
	s.streams = make(map[uint32]*Stream)
	s.chAccepts = make(chan *Stream, defaultAcceptBacklog)
	s.bucket = int32(config.MaxReceiveBuffer)
	s.bucketNotify = make(chan struct{}, 1)
	s.shaper = make(chan writeRequest)
	s.writes = make(chan writeRequest)
	s.chSocketReadError = make(chan struct{})
	s.chSocketWriteError = make(chan struct{})
	s.chProtoError = make(chan struct{})

	if client {
		s.nextStreamID = 1
	} else {
		s.nextStreamID = 0
	}

	go s.shaperLoop()
	go s.recvLoop()
	go s.sendLoop()
	if !config.KeepAliveDisabled {
		go s.keepalive()
	}
	return s
}

// OpenStream is used to create a new stream
func (s *Session) OpenStream() (*Stream, error) {
	return s.openStreamInternal(false)
}

func (s *Session) openStreamInternal(immediate bool) (*Stream, error) {
	if s.IsClosed() {
		return nil, io.ErrClosedPipe
	}

	// generate stream id
	s.nextStreamIDLock.Lock()
	if s.goAway > 0 {
		s.nextStreamIDLock.Unlock()
		return nil, ErrGoAway
	}

	s.nextStreamID += 2
	sid := s.nextStreamID
	if sid == sid%2 { // stream-id overflows
		s.goAway = 1
		s.nextStreamIDLock.Unlock()
		return nil, ErrGoAway
	}
	s.nextStreamIDLock.Unlock()

	stream := newStream(sid, s.config.MaxFrameSize, s)

	if _, err := s.writeFrameInternal(newFrame(byte(s.config.Version), cmdSYN, sid), nil, 0, immediate); err != nil {
		return nil, err
	}

	s.streamLock.Lock()
	defer s.streamLock.Unlock()
	select {
	case <-s.chSocketReadError:
		return nil, s.socketReadError.Load().(error)
	case <-s.chSocketWriteError:
		return nil, s.socketWriteError.Load().(error)
	case <-s.die:
		return nil, io.ErrClosedPipe
	default:
		s.streams[sid] = stream
		return stream, nil
	}
}

// Open returns a generic ReadWriteCloser
func (s *Session) Open() (io.ReadWriteCloser, error) {
	return s.OpenStream()
}

// AcceptStream is used to block until the next available stream
// is ready to be accepted.
func (s *Session) AcceptStream() (*Stream, error) {
	var deadline <-chan time.Time
	if d, ok := s.deadline.Load().(time.Time); ok && !d.IsZero() {
		timer := time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	select {
	case stream := <-s.chAccepts:
		return stream, nil
	case <-deadline:
		return nil, ErrTimeout
	case <-s.chSocketReadError:
		return nil, s.socketReadError.Load().(error)
	case <-s.chProtoError:
		return nil, s.protoError.Load().(error)
	case <-s.die:
		return nil, io.ErrClosedPipe
	}
}

// Accept Returns a generic ReadWriteCloser instead of smux.Stream
func (s *Session) Accept() (io.ReadWriteCloser, error) {
	return s.AcceptStream()
}

// Close is used to close the session and all streams.
func (s *Session) Close() error {
	var once bool
	s.dieOnce.Do(func() {
		close(s.die)
		once = true
	})

	if once {
		s.streamLock.Lock()
		for k := range s.streams {
			s.streams[k].sessionClose()
		}
		s.streamLock.Unlock()
		return s.conn.Close()
	} else {
		return io.ErrClosedPipe
	}
}

// notifyBucket notifies recvLoop that bucket is available
func (s *Session) notifyBucket() {
	select {
	case s.bucketNotify <- struct{}{}:
	default:
	}
}

func (s *Session) notifyReadError(err error) {
	s.socketReadErrorOnce.Do(func() {
		s.socketReadError.Store(err)
		close(s.chSocketReadError)
	})
}

func (s *Session) notifyWriteError(err error) {
	s.socketWriteErrorOnce.Do(func() {
		s.socketWriteError.Store(err)
		close(s.chSocketWriteError)
	})
}

func (s *Session) notifyProtoError(err error) {
	s.protoErrorOnce.Do(func() {
		s.protoError.Store(err)
		close(s.chProtoError)
	})
}

// IsClosed does a safe check to see if we have shutdown
func (s *Session) IsClosed() bool {
	select {
	case <-s.die:
		return true
	default:
		return false
	}
}

// NumStreams returns the number of currently open streams
func (s *Session) NumStreams() int {
	if s.IsClosed() {
		return 0
	}
	s.streamLock.Lock()
	defer s.streamLock.Unlock()
	return len(s.streams)
}

// SetDeadline sets a deadline used by Accept* calls.
// A zero time value disables the deadline.
func (s *Session) SetDeadline(t time.Time) error {
	s.deadline.Store(t)
	return nil
}

// LocalAddr satisfies net.Conn interface
func (s *Session) LocalAddr() net.Addr {
	if ts, ok := s.conn.(interface {
		LocalAddr() net.Addr
	}); ok {
		return ts.LocalAddr()
	}
	return nil
}

// RemoteAddr satisfies net.Conn interface
func (s *Session) RemoteAddr() net.Addr {
	if ts, ok := s.conn.(interface {
		RemoteAddr() net.Addr
	}); ok {
		return ts.RemoteAddr()
	}
	return nil
}

// notify the session that a stream has closed
func (s *Session) streamClosed(sid uint32) {
	s.streamLock.Lock()
	if n := s.streams[sid].recycleTokens(); n > 0 { // return remaining tokens to the bucket
		if atomic.AddInt32(&s.bucket, int32(n)) > 0 {
			s.notifyBucket()
		}
	}
	delete(s.streams, sid)
	s.streamLock.Unlock()
}

// returnTokens is called by stream to return token after read
func (s *Session) returnTokens(n int) {
	if atomic.AddInt32(&s.bucket, int32(n)) > 0 {
		s.notifyBucket()
	}
}

// recvLoop keeps on reading from underlying connection if tokens are available
func (s *Session) recvLoop() {
	var hdr rawHeader
	var updHdr updHeader

	for {
		for atomic.LoadInt32(&s.bucket) <= 0 && !s.IsClosed() {
			select {
			case <-s.bucketNotify:
			case <-s.die:
				return
			}
		}

		// read header first
		if _, err := io.ReadFull(s.conn, hdr[:]); err == nil {
			atomic.StoreInt32(&s.dataReady, 1)
			if hdr.Version() != byte(s.config.Version) {
				s.notifyProtoError(ErrInvalidProtocol)
				return
			}
			sid := hdr.StreamID()
			switch hdr.Cmd() {
			case cmdNOP:
				padding := int(hdr.Length())
				if padding > 0 {
					newbuf := defaultAllocator.Get(padding)
					_, err := io.ReadFull(s.conn, newbuf)
					defaultAllocator.Put(newbuf)
					if err != nil {
						s.notifyReadError(err)
						return
					}
				}
			case cmdSYN:
				s.streamLock.Lock()
				if _, ok := s.streams[sid]; !ok {
					stream := newStream(sid, s.config.MaxFrameSize, s)
					s.streams[sid] = stream
					select {
					case s.chAccepts <- stream:
					case <-s.die:
					}
				}
				s.streamLock.Unlock()
			case cmdFIN:
				s.streamLock.Lock()
				if stream, ok := s.streams[sid]; ok {
					stream.fin()
					stream.notifyReadEvent()
				}
				s.streamLock.Unlock()
			case cmdPSH:
				if hdr.Length() > 0 {
					newbuf := defaultAllocator.Get(int(hdr.Length()))
					if written, err := io.ReadFull(s.conn, newbuf); err == nil {
						s.streamLock.Lock()
						if stream, ok := s.streams[sid]; ok {
							stream.pushBytes(newbuf)
							atomic.AddInt32(&s.bucket, -int32(written))
							stream.notifyReadEvent()
						}
						s.streamLock.Unlock()
					} else {
						s.notifyReadError(err)
						return
					}
				}
			case cmdUPD:
				if _, err := io.ReadFull(s.conn, updHdr[:]); err == nil {
					s.streamLock.Lock()
					if stream, ok := s.streams[sid]; ok {
						stream.update(updHdr.Consumed(), updHdr.Window())
					}
					s.streamLock.Unlock()
				} else {
					s.notifyReadError(err)
					return
				}
			default:
				s.notifyProtoError(ErrInvalidProtocol)
				return
			}
		} else {
			s.notifyReadError(err)
			return
		}
	}
}

func (s *Session) keepalive() {
	tickerPing := time.NewTicker(s.config.KeepAliveInterval)
	tickerTimeout := time.NewTicker(s.config.KeepAliveTimeout)
	defer tickerPing.Stop()
	defer tickerTimeout.Stop()
	for {
		select {
		case <-tickerPing.C:
			s.writeFrameInternal(newFrame(byte(s.config.Version), cmdNOP, 0), tickerPing.C, 0, true)
			s.notifyBucket() // force a signal to the recvLoop
		case <-tickerTimeout.C:
			if !atomic.CompareAndSwapInt32(&s.dataReady, 1, 0) {
				// recvLoop may block while bucket is 0, in this case,
				// session should not be closed.
				if atomic.LoadInt32(&s.bucket) > 0 {
					s.Close()
					return
				}
			}
		case <-s.die:
			return
		}
	}
}

// shaper shapes the sending sequence among streams
func (s *Session) shaperLoop() {
	var reqs shaperHeap
	var next writeRequest
	var chWrite chan writeRequest

	for {
		if len(reqs) > 0 {
			chWrite = s.writes
			next = heap.Pop(&reqs).(writeRequest)
		} else {
			chWrite = nil
		}

		select {
		case <-s.die:
			return
		case r := <-s.shaper:
			if chWrite != nil { // next is valid, reshape
				heap.Push(&reqs, next)
			}
			heap.Push(&reqs, r)
		case chWrite <- next:
		}
	}
}

func (s *Session) sendLoop() {
	buf := make([]byte, 0, (1<<16)+headerSize)
	hbuf := make([]byte, headerSize)
	ackList := make([]writeRequest, 0, 8)

	// the buffer is allowed to grow beyond this size to service
	// very large writes, but we will not keep coalescing after the
	// buffer is "full" with respect to its initial size.
	flushMax := cap(buf)

	// the minimum deliverable padding frame size.
	// since at least a header (with 0 data length) is
	// required, total padding cannot be less than the
	// header size.
	minPadding := headerSize
	ratio := s.config.MaxPaddingRatio
	aggRatio := s.config.AggressivePaddingRatio
	maxPaddedSize := s.config.MaxPaddedSize
	maxPadding := int(math.Floor(math.Max(ratio, aggRatio) * float64(maxPaddedSize)))
	if maxPadding > maxPaddedSize {
		maxPadding = maxPaddedSize
	}
	if maxPadding < minPadding {
		maxPadding = minPadding
	}

	padBuf := make([]byte, maxPadding)
	padBuf[0] = byte(s.config.Version)
	padBuf[1] = cmdNOP
	aggCount := 0

	_pack := func(request writeRequest) {
		sz := len(request.frame.data)
		hbuf[0] = request.frame.ver
		hbuf[1] = request.frame.cmd
		binary.LittleEndian.PutUint16(hbuf[2:], uint16(sz))
		binary.LittleEndian.PutUint32(hbuf[4:], request.frame.sid)
		buf = append(buf, hbuf...)
		buf = append(buf, request.frame.data...)
		ackList = append(ackList, request)
	}

	_ack := func(err error) {
		for _, request := range ackList {
			if err != nil {
				request.result <- writeResult{0, err}
			} else {
				request.result <- writeResult{len(request.frame.data), nil}
			}
			close(request.result)
		}
		ackList = ackList[:0]
	}

	_maybePad := func() error {
		pmax := maxPaddedSize - len(buf)

		// pick normal or aggressive padding ratio
		// depending on number of padded writes performed
		// in session
		r := ratio
		if aggCount < s.config.AggressivePadding {
			r = aggRatio
			aggCount += 1
		}

		// limit max padding size by padding ratio
		if rmax := int(math.Floor(float64(len(buf)) * r)); rmax < pmax {
			pmax = rmax
		}

		var padSize int
		if pmax > minPadding {
			// randomly select padding size up to limit
			pbig, err := rand.Int(rand.Reader, big.NewInt(int64(pmax-minPadding)))
			if err != nil {
				return err
			}
			padSize = int(pbig.Int64()) + minPadding
		} else if pmax == minPadding {
			padSize = minPadding
		} else { // pmax < minPadding
			return nil
		}

		binary.LittleEndian.PutUint16(padBuf[2:], uint16(padSize-headerSize))
		buf = append(buf, padBuf[:padSize]...)

		return nil
	}

	for {
		select {
		case <-s.die:
			return
		case request := <-s.writes:
			_pack(request)
			if request.flush || len(buf) >= flushMax {
				// maybe coalesce additional pending writes ...
				done := false
				for !done && len(buf) < flushMax {
					// give other goroutines a shot at writing by
					// explicitly yielding for a moment before
					// testing for additional pending writes.
					// a small linger of 1ms or so could also
					// be potentially useful for coalescing writes,
					// but high resolution timers are a bit funky
					// on windows and require system level changes to
					// timer resolution that affect power consumption and
					// other running programs. May eventually be helped by this
					// https://go-review.googlesource.com/c/go/+/248699/
					// but not sure it applies to everything necessary...
					runtime.Gosched()
					select {
					case request = <-s.writes:
						_pack(request)
					default:
						done = true
					}
				}
				err := _maybePad()
				if err != nil {
					s.notifyWriteError(err)
					_ack(err)
					return
				}

				// flush
				_, err = s.conn.Write(buf)
				buf = buf[:0]
				if err != nil {
					s.notifyWriteError(err)
					_ack(err)
					return
				}
			}
			_ack(nil)
		}
	}
}

// writeFrame writes the frame to the underlying connection
// and returns the number of bytes written if successful
func (s *Session) writeFrame(f Frame) (n int, err error) {
	return s.writeFrameInternal(f, nil, 0, false)
}

// internal writeFrame version to support deadline used in keepalive
func (s *Session) writeFrameInternal(f Frame, deadline <-chan time.Time, prio uint64, flush bool) (int, error) {
	req := writeRequest{
		prio:   prio,
		frame:  f,
		result: make(chan writeResult, 1),
		flush:  flush,
	}
	select {
	case s.shaper <- req:
	case <-s.die:
		return 0, io.ErrClosedPipe
	case <-s.chSocketWriteError:
		return 0, s.socketWriteError.Load().(error)
	case <-deadline:
		return 0, ErrTimeout
	}

	select {
	case result := <-req.result:
		return result.n, result.err
	case <-s.die:
		return 0, io.ErrClosedPipe
	case <-s.chSocketWriteError:
		return 0, s.socketWriteError.Load().(error)
	case <-deadline:
		return 0, ErrTimeout
	}
}
