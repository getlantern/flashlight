// Package logging configures the log subsystem for use with Lantern
// Import this to make sure logging is initialized before you log.
package logging

import (
	"io"
	"runtime"

	log "github.com/sirupsen/logrus"

	"github.com/getlantern/flashlight/borda"
)

func init() {
	if runtime.GOOS != "android" {
		EnableFileLogging("")
	}
}

func EnableFileLogging(logdir string) {
	log.AddHook(borda.NewLogrusHook())
}

// ZipLogFiles zip the Lantern log files to the writer. All files will be
// placed under the folder in the archieve.  It will stop and return if the
// newly added file would make the extracted files exceed maxBytes in total.
func ZipLogFiles(w io.Writer, underFolder string, maxBytes int64) error {
	/*
		actualLogDirMx.RLock()
		logdir := actualLogDir
		actualLogDirMx.RUnlock()

		return util.ZipFiles(w, util.ZipOptions{
			Glob:     "lantern.log*",
			Dir:      logdir,
			NewRoot:  underFolder,
			MaxBytes: maxBytes,
		})
	*/
	return nil
}

// Close stops logging.
func Close() error {
	return nil
}
