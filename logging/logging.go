// Package logging configures the log subsystem for use with Lantern
// Import this to make sure logging is initialized before you log.
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	log "github.com/sirupsen/logrus"

	"github.com/getlantern/appdir"
	"github.com/getlantern/flashlight/borda"
	"github.com/getlantern/flashlight/common"
)

const logTimestampFormat = "Jan 02 15:04:05.000"

func init() {
	if runtime.GOOS != "android" {
		enableFileLogging()
	}
}

func enableFileLogging() {
	fmt.Println("ENABLING FILE LOGGING")
	logdir := appdir.Logs("Lantern")

	filename := filepath.Join(logdir, "lantern.log")
	logFile, err := os.OpenFile(filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return
	}

	mw := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(mw)
	log.SetLevel(log.InfoLevel)
	formatter := &log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: logTimestampFormat,
		DisableColors:   true,
	}

	if common.InDevel() {
		//formatter.ForceColors = true
		//formatter.DisableColors = false
	}
	log.SetFormatter(formatter)
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
