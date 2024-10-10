// Package logging configures the golog subsystem for use with Lantern
// Import this to make sure golog is initialized before you log.
package logging

import (
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/rotator"

	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/util"
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

	resetLogs atomic.Value
)

func init() {
	resetLogs.Store(func() {})
}

// FlashlightLogger is a logger that uses golog.Logger to log messages. It
// implements a logging interface that packages like quicproxy use.
// See here for more info:
// - https://github.com/getlantern/quicproxy/blob/d393da079842dda222d5c0ddbc1ba33e55c46e8b/README.md#L40
type FlashlightLogger struct{ golog.Logger }

func (l FlashlightLogger) Printf(format string, a ...any) { l.Logger.Debugf(format, a...) }
func (l FlashlightLogger) Infof(format string, a ...any)  { l.Logger.Debugf(format, a...) }
func (l FlashlightLogger) Errorf(format string, a ...any) { l.Logger.Errorf(format, a...) }

// RotatedLogsUnder creates rotated file logger under logdir using the given appName
func RotatedLogsUnder(appName, logdir string) (io.WriteCloser, error) {
	actualLogDirMx.Lock()
	actualLogDir = logdir
	actualLogDirMx.Unlock()

	if _, err := os.Stat(logdir); err != nil {
		if os.IsNotExist(err) {
			// Create log dir
			if err := os.MkdirAll(logdir, 0755); err != nil {
				return nil, errors.New("Unable to create logdir at %s: %s", logdir, err)
			}
		}
	}

	rotator := rotator.NewSizeRotator(filepath.Join(logdir, strings.ToLower(appName)+".log"))
	// Set log files to 4 MB
	rotator.RotationSize = 4 * 1024 * 1024
	// Keep up to 5 log files
	rotator.MaxRotation = 5

	return rotator, nil
}

// Timestamped writes the current time and the duration since process start to
// the writer, used by golog.SetPrepender().
func Timestamped(w io.Writer) {
	ts := time.Now()
	runningSecs := ts.Sub(processStart).Seconds()
	secs := int(math.Mod(runningSecs, 60))
	mins := int(runningSecs / 60)
	fmt.Fprintf(w, "%s - %dm%ds ", ts.In(time.UTC).Format(logTimestampFormat), mins, secs)
}

// EnableFileLogging configures golog to write to rotated files under the
// logdir for the given appName, in addition to standard outputs.
func EnableFileLogging(appName, logdir string) {
	err := EnableFileLoggingWith(os.Stdout, os.Stderr, appName, logdir, 100, 1000)
	if err != nil {
		log.Error(err)
	}
}

// EnableFileLoggingWith is similar to EnableFileLogging but allows overriding
// standard outputs and setting buffer depths for error and debug log channels.
func EnableFileLoggingWith(werr io.WriteCloser, wout io.WriteCloser, appName, logdir string, debugBufferDepth, errorBufferDepth int) error {
	golog.SetPrepender(Timestamped)
	rotator, err := RotatedLogsUnder(appName, logdir)
	if err != nil {
		return err
	}

	logFile = rotator
	errorPWC = newPipedWriteCloser(NonStopWriteCloser(werr, logFile), errorBufferDepth)
	debugPWC = newPipedWriteCloser(NonStopWriteCloser(wout, logFile), debugBufferDepth)
	resetLogs.Store(golog.SetOutputs(errorPWC, debugPWC))
	return nil
}

type pipedWriteCloser struct {
	nSkipped uint64
	closing  uint32 // 1 means true
	w        io.WriteCloser
	ch       chan []byte
	chClosed chan struct{}
}

func (w *pipedWriteCloser) Write(b []byte) (int, error) {
	if atomic.LoadUint32(&w.closing) > 0 {
		return len(b), nil
	}
	buf := make([]byte, len(b))
	// Have to copy the slice as the caller may reuse it before it's consumed
	// by the write goroutine.
	copy(buf, b)
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
	}
	go func() {
		for b := range pwc.ch {
			pwc.w.Write(b)
		}
		close(pwc.chClosed)
	}()
	return pwc
}

// ZipLogFiles zips the Lantern log files to the writer. All files will be
// placed under the folder in the archieve.  It will stop and return if the
// newly added file would make the extracted files exceed maxBytes in total.
//
// It also returns up to maxTextBytes of plain text from the end of the most recent log file.
func ZipLogFiles(w io.Writer, underFolder string, maxBytes int64, maxTextBytes int64) (string, error) {
	actualLogDirMx.RLock()
	logdir := actualLogDir
	actualLogDirMx.RUnlock()
	if common.Platform != "ios" {
		return ZipLogFilesFrom(w, maxBytes, maxTextBytes, map[string]string{"logs": logdir})
	}
	// On iOS, for the tunnel process running in a different directory,
	// we need to zip those logs as well.
	tunnelLogDir := strings.Replace(logdir, "/app/", "/netEx/", 1)
	return ZipLogFilesFrom(w, maxBytes, maxTextBytes, map[string]string{"logs": logdir, "tunnelLogs": tunnelLogDir})
}

// ZipLogFilesFrom zips the log files from the given dirs to the writer. It will
// stop and return if the newly added file would make the extracted files exceed
// maxBytes in total.
//
// It also returns up to maxTextBytes of plain text from the end of the most recent log file.
func ZipLogFilesFrom(w io.Writer, maxBytes int64, maxTextBytes int64, dirs map[string]string) (string, error) {
	globs := make(map[string]string, len(dirs))
	for baseDir, dir := range dirs {
		globs[baseDir] = fmt.Sprintf(filepath.Join(dir, "*"))
	}
	err := util.ZipFiles(w, util.ZipOptions{
		Globs:    globs,
		MaxBytes: maxBytes,
	})
	if err != nil {
		return "", err
	}

	if maxTextBytes <= 0 {
		return "", nil
	}

	// Get info for all log files
	allFiles := make(byDate, 0)
	for _, glob := range globs {
		matched, err := filepath.Glob(glob)
		if err != nil {
			log.Errorf("Unable to list files at glob %v: %v", glob, err)
			continue
		}
		for _, file := range matched {
			fi, err := os.Stat(file)
			if err != nil {
				log.Errorf("Unable to stat file %v: %v", file, err)
				continue
			}
			allFiles = append(allFiles, &fileInfo{
				file:    file,
				size:    fi.Size(),
				modTime: fi.ModTime().Unix(),
			})
		}
	}

	if len(allFiles) > 0 {
		// Sort by recency
		sort.Sort(allFiles)

		mostRecent := allFiles[0]
		log.Debugf("Grabbing log tail from %v", mostRecent.file)

		mostRecentFile, err := os.Open(mostRecent.file)
		if err != nil {
			log.Errorf("Unable to open most recent log file %v: %v", mostRecent.file, err)
			return "", nil
		}
		defer mostRecentFile.Close()

		seekTo := mostRecent.size - maxTextBytes
		if seekTo > 0 {
			log.Debugf("Seeking to %d in %v", seekTo, mostRecent.file)
			_, err = mostRecentFile.Seek(seekTo, os.SEEK_CUR)
			if err != nil {
				log.Errorf("Unable to seek to tail of file %v: %v", mostRecent.file, err)
				return "", nil
			}
		}
		tail, err := ioutil.ReadAll(mostRecentFile)
		if err != nil {
			log.Errorf("Unable to read tail of file %v: %v", mostRecent.file, err)
			return "", nil
		}

		log.Debugf("Got %d bytes of log tail from %v", len(tail), mostRecent.file)
		return string(tail), nil
	}

	return "", nil
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
	resetLogs.Load().(func())()

	return nil
}

type nonStopWriteCloser struct {
	writers []io.WriteCloser
}

// NonStopWriteCloser creates a WriteCloser that duplicates its writes to all
// the provided WriteClosers, even if errors encountered while writing. It
// doesn't close the provided WriteClosers.
func NonStopWriteCloser(writers ...io.WriteCloser) io.WriteCloser {
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

type fileInfo struct {
	file    string
	size    int64
	modTime int64
}
type byDate []*fileInfo

func (a byDate) Len() int           { return len(a) }
func (a byDate) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byDate) Less(i, j int) bool { return a[i].modTime > a[j].modTime }
