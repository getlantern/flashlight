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
	"sync"
	"time"

	"github.com/getlantern/appdir"
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

	logFile io.WriteCloser

	errorOut io.Writer
	debugOut io.Writer

	actualLogDir   string
	actualLogDirMx sync.RWMutex
)

func init() {
	if runtime.GOOS != "android" {
		EnableFileLogging("")
	}
}

func EnableFileLogging(logdir string) {
	if logdir == "" {
		logdir = appdir.Logs("Lantern")
	}
	actualLogDirMx.Lock()
	actualLogDir = logdir
	actualLogDirMx.Unlock()

	log.Debugf("Placing logs in %v", logdir)
	if _, err := os.Stat(logdir); err != nil {
		if os.IsNotExist(err) {
			// Create log dir
			if err := os.MkdirAll(logdir, 0755); err != nil {
				log.Errorf("Unable to create logdir at %s: %s", logdir, err)
				return
			}
		}
	}
	rotator := rotator.NewSizeRotator(filepath.Join(logdir, "lantern.log"))
	// Set log files to 4 MB
	rotator.RotationSize = 4 * 1024 * 1024
	// Keep up to 5 log files
	rotator.MaxRotation = 5

	logFile = newPipedWriteCloser(rotator, 10000)
	errorOut = timestamped(newNonStopWriter(os.Stderr, logFile))
	debugOut = timestamped(newNonStopWriter(os.Stdout, logFile))
	golog.SetOutputs(errorOut, debugOut)
}

type pipedWriteCloser struct {
	w        io.WriteCloser
	ch       chan []byte
	chClosed chan bool
	bufPool  sync.Pool // to reduce allocation as much as possible
}

func (w *pipedWriteCloser) Write(b []byte) (int, error) {
	buf := w.bufPool.Get().([]byte)
	// Have to copy the slice as the caller may reuse it before it's consumed
	// by the write goroutine. Using append is a trick to grow the slice
	// automatically.
	buf = append(buf[:0], b...)
	select {
	case w.ch <- buf:
	default:
	}
	return len(b), nil
}

func (w *pipedWriteCloser) Close() error {
	close(w.ch)
	<-w.chClosed
	return w.w.Close()
}

// newPipedWriteCloser wraps a WriteCloser to sequentialize writes from
// different goroutines into a single goroutine. Write errors won't be
// propagated back to the caller goroutine and pending writes more than
// nPending will be dropped silently.
func newPipedWriteCloser(w io.WriteCloser, nPending int) io.WriteCloser {
	pwc := &pipedWriteCloser{w,
		make(chan []byte, nPending),
		make(chan bool),
		sync.Pool{
			New: func() interface{} { return make([]byte, 0, 256) },
		},
	}
	go func() {
		for b := range pwc.ch {
			pwc.w.Write(b)
			pwc.bufPool.Put(b)
		}
		pwc.chClosed <- true
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

	return util.ZipFiles(w, util.ZipOptions{
		Glob:     "lantern.log*",
		Dir:      logdir,
		NewRoot:  underFolder,
		MaxBytes: maxBytes,
	})
}

// Close stops logging.
func Close() error {
	initLogging()
	if logFile != nil {
		return logFile.Close()
	}
	return nil
}

func initLogging() {
	errorOut = timestamped(os.Stderr)
	debugOut = timestamped(os.Stdout)
	golog.SetOutputs(errorOut, debugOut)
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

type nonStopWriter struct {
	writers []io.Writer
}

// newNonStopWriter creates a writer that duplicates its writes to all the
// provided writers, even if errors encountered while writting.
func newNonStopWriter(writers ...io.Writer) io.Writer {
	w := make([]io.Writer, len(writers))
	copy(w, writers)
	return &nonStopWriter{w}
}

// Write implements the method from io.Writer.
// It never fails and always return the length of bytes passed in
func (t *nonStopWriter) Write(p []byte) (int, error) {
	for _, w := range t.writers {
		// intentionally not checking for errors
		_, _ = w.Write(p)
	}
	return len(p), nil
}
