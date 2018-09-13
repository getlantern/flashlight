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
	"go.uber.org/zap"

	"github.com/getlantern/flashlight/util"
)

const (
	logTimestampFormat = "Jan 02 15:04:05.000"
)

var (
	log          = zap.NewExample().Sugar()
	processStart = time.Now()

	logFile *rotator.SizeRotator

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
	logFile = rotator.NewSizeRotator(filepath.Join(logdir, "lantern.log"))
	// Set log files to 4 MB
	logFile.RotationSize = 4 * 1024 * 1024
	// Keep up to 5 log files
	logFile.MaxRotation = 5

	errorOut = timestamped(NonStopWriter(os.Stderr, logFile))
	debugOut = timestamped(NonStopWriter(os.Stdout, logFile))
	golog.SetOutputs(errorOut, debugOut)
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

// NonStopWriter creates a writer that duplicates its writes to all the
// provided writers, even if errors encountered while writting.
func NonStopWriter(writers ...io.Writer) io.Writer {
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
