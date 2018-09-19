// Package logging configures the logging subsystem for use with Lantern
// Import this to make sure logging is initialized before you log.
package logging

import (
	"io"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/getlantern/appdir"
	"github.com/getlantern/rotator"
	"github.com/getlantern/zaplog"
	"go.uber.org/zap"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/util"
)

var (
	log = zaplog.LoggerFor("flashlight.logging")

	logFile *rotator.SizeRotator

	actualLogDir   string
	actualLogDirMx sync.RWMutex
)

func init() {
	if runtime.GOOS != "android" {
		EnableFileLogging("")
	}
}

// EnableFileLogging enables logging at the specified path. Uses the default
// OS-sepcific path is logdir is empty.
func EnableFileLogging(logdir string) {
	if logdir == "" {
		logdir = appdir.Logs("Lantern")
	}
	actualLogDirMx.Lock()
	actualLogDir = logdir
	actualLogDirMx.Unlock()

	logPath := filepath.Join(logdir, "lantern.log")
	logFile = rotator.NewSizeRotator(logPath)
	// Set log files to 4 MB
	logFile.RotationSize = 4 * 1024 * 1024
	// Keep up to 5 log files
	logFile.MaxRotation = 5

	var config zap.Config
	var zapOutPaths []string
	if common.IsDevel() {
		config = zap.NewDevelopmentConfig()
		zapOutPaths = []string{"stderr", "lantern.log"}
	} else {
		config = zap.NewProductionConfig()
		zapOutPaths = []string{logPath}
	}
	config.OutputPaths = zapOutPaths
	zaplog.SetZapConfig(config)

	log.Infof("Placing logs in %v", logdir)
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
	if logFile != nil {
		return logFile.Close()
	}
	zaplog.Close()
	return nil
}
