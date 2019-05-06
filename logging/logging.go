// package logging configures the golog subsystem for use with Lantern
// Import this to make sure golog is initialized before you log.
package logging

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/rotator"
	"github.com/getlantern/wfilter"

	"github.com/getlantern/flashlight/util"
)

const (
	logTimestampFormat = "Jan 02 15:04:05.000"
)

var (
	log          = golog.LoggerFor("flashlight.logging")
	processStart = time.Now()

	logFile  io.WriteCloser
	errorPWC io.WriteCloser
	debugPWC io.WriteCloser

	actualLogDir   string
	actualLogDirMx sync.RWMutex
)

func init() {
	if !strings.HasPrefix(runtime.GOARCH, "arm") {
		EnableFileLogging("")
	}
}

func EnableFileLogging(logdir string) {
	err := EnableFileLoggingWith(os.Stdout, os.Stderr, logdir)
	if err != nil {
		log.Error(err)
	}
}

func EnableFileLoggingWith(werr io.WriteCloser, wout io.WriteCloser, logdir string) error {
	if logdir == "" {
		logdir = appdir.Logs("Lantern")
	}
	actualLogDirMx.Lock()
	actualLogDir = logdir
	actualLogDirMx.Unlock()

	if _, err := os.Stat(logdir); err != nil {
		if os.IsNotExist(err) {
			// Create log dir
			if err := os.MkdirAll(logdir, 0755); err != nil {
				return errors.New("Unable to create logdir at %s: %s", logdir, err)
			}
		}
	}
	rotator := rotator.NewSizeRotator(filepath.Join(logdir, "lantern.log"))
	// Set log files to 4 MB
	rotator.RotationSize = 4 * 1024 * 1024
	// Keep up to 5 log files
	rotator.MaxRotation = 5

	logFile = rotator
	errorPWC = newPipedWriteCloser(newNonStopWriteCloser(werr, logFile), 1000)
	debugPWC = newPipedWriteCloser(newNonStopWriteCloser(wout, logFile), 100)
	golog.SetOutputs(timestamped(errorPWC), timestamped(debugPWC))
	return nil
}

type pipedWriteCloser struct {
	nSkipped uint64
	closing  uint32 // 1 means true
	w        io.WriteCloser
	ch       chan []byte
	chClosed chan struct{}
	bufPool  sync.Pool // to reduce allocation as much as possible
}

func (w *pipedWriteCloser) Write(b []byte) (int, error) {
	if atomic.LoadUint32(&w.closing) > 0 {
		return len(b), nil
	}
	buf := w.bufPool.Get().([]byte)
	// Have to copy the slice as the caller may reuse it before it's consumed
	// by the write goroutine. Using append is a trick to grow the slice
	// automatically.
	buf = append(buf[:0], b...)
	select {
	case w.ch <- buf:
		skipped := atomic.LoadUint64(&w.nSkipped)
		if skipped > 0 {
			select {
			case w.ch <- []byte(fmt.Sprintf("...%d message(s) skipped...\n", skipped)):
				// Note: Race condition could cause the message being printed
				// several times, but that's acceptable.
				atomic.StoreUint64(&w.nSkipped, 0)
			default:
			}
		}
	default:
		// Intentionally not returning the buffer to pool, to prevent the pool
		// from expanding indefinitely.
		atomic.AddUint64(&w.nSkipped, 1)
	}
	return len(b), nil
}

func (w *pipedWriteCloser) Close() error {
	if !atomic.CompareAndSwapUint32(&w.closing, 0, 1) {
		// Closing in progress or done
		return nil
	}
	close(w.ch)
	<-w.chClosed
	return w.w.Close()
}

// newPipedWriteCloser wraps a WriteCloser to sequentialize writes from
// different goroutines into a single goroutine. Write errors won't be
// propagated back to the caller goroutine and pending writes more than
// nPending will be dropped silently.
func newPipedWriteCloser(w io.WriteCloser, nPending int) io.WriteCloser {
	pwc := &pipedWriteCloser{0, 0, w,
		make(chan []byte, nPending),
		make(chan struct{}),
		sync.Pool{
			New: func() interface{} { return make([]byte, 0, 256) },
		},
	}
	go func() {
		for b := range pwc.ch {
			pwc.w.Write(b)
			pwc.bufPool.Put(b)
		}
		close(pwc.chClosed)
	}()
	return pwc
}

// ZipLogFiles zip the Lantern log files to the writer. All files will be
// placed under the folder in the archieve.  It will stop and return if the
// newly added file would make the extracted files exceed maxBytes in total.
func ZipLogFiles(w io.Writer, underFolder string, maxBytes int64) error {
	actualLogDirMx.RLock()
	logdir := actualLogDir
	actualLogDirMx.RUnlock()

	return ZipLogFilesFrom(w, maxBytes, map[string]string{"logs": logdir})
}

// ZipLogFilesFrom zip the log files from the given dirs to the writer. It will
// stop and return if the newly added file would make the extracted files exceed
// maxBytes in total.
func ZipLogFilesFrom(w io.Writer, maxBytes int64, dirs map[string]string) error {
	globs := make(map[string]string, len(dirs))
	for baseDir, dir := range dirs {
		globs[baseDir] = fmt.Sprintf(filepath.Join(dir, "*.log*"))
	}
	return util.ZipFiles(w, util.ZipOptions{
		Globs:    globs,
		MaxBytes: maxBytes,
	})
}

// Close stops logging.
func Close() error {
	if errorPWC != nil {
		errorPWC.Close()
	}
	if debugPWC != nil {
		debugPWC.Close()
	}
	if logFile != nil {
		logFile.Close()
	}
	initLogging()
	return nil
}

func initLogging() {
	golog.SetOutputs(timestamped(os.Stderr), timestamped(os.Stdout))
}

// timestamped adds a timestamp to the beginning of log lines
func timestamped(orig io.Writer) io.Writer {
	return wfilter.SimplePrepender(orig, func(w io.Writer) (int, error) {
		ts := time.Now()
		runningSecs := ts.Sub(processStart).Seconds()
		secs := int(math.Mod(runningSecs, 60))
		mins := int(runningSecs / 60)
		return fmt.Fprintf(w, "%s - %dm%ds ", ts.In(time.UTC).Format(logTimestampFormat), mins, secs)
	})
}

type nonStopWriteCloser struct {
	writers []io.WriteCloser
}

// newNonStopWriteCloser creates a WriteCloser that duplicates its writes to all the
// provided WriteClosers, even if errors encountered while writing. It doesn't
// close the provided WriteClosers.
func newNonStopWriteCloser(writers ...io.WriteCloser) io.WriteCloser {
	w := make([]io.WriteCloser, len(writers))
	copy(w, writers)
	return &nonStopWriteCloser{w}
}

// Write implements the method from io.Writer.
// It never fails and always return the length of bytes passed in
func (t *nonStopWriteCloser) Write(p []byte) (int, error) {
	for _, w := range t.writers {
		// intentionally not checking for errors
		_, _ = w.Write(p)
	}
	return len(p), nil
}

func (t *nonStopWriteCloser) Close() error {
	return nil
}
