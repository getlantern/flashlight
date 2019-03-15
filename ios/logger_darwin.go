package ios

// #include <os/log.h>
//
// void log_debug(const char *msg)
// {
//   os_log(OS_LOG_DEFAULT, "%{public}s", msg);
// }
//
// void log_error(const char *msg)
// {
//   os_log(OS_LOG_DEFAULT, "%{public}s", msg);
// }
import "C"

import (
	"io"
	"os"
	"path/filepath"

	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/golog"
)

func init() {
	golog.SetOutputs(
		io.MultiWriter(os.Stderr, loggerWith(func(msg string) {
			C.log_error(C.CString(msg))
		})),
		io.MultiWriter(os.Stdout, loggerWith(func(msg string) {
			C.log_debug(C.CString(msg))
		})))
}

// ConfigureFileLogging configures file logging to use the lantern.log file at
// the given path. It is required that the file, as well as files lantern.log.1
// through lantern.log.5 already exist so that they are writeable from the Go
// side.
func ConfigureFileLogging(fullLogFilePath string) error {
	logFileDirectory, _ := filepath.Split(fullLogFilePath)
	return logging.EnableFileLoggingWith(loggerWith(func(msg string) {
		C.log_error(C.CString(msg))
	}), loggerWith(func(msg string) {
		C.log_debug(C.CString(msg))
	}), logFileDirectory)
}

func loggerWith(fn func(string)) io.WriteCloser {
	return &loggerFn{fn}
}

type loggerFn struct {
	log func(string)
}

func (lf *loggerFn) Write(msg []byte) (int, error) {
	lf.log(string(msg))
	return len(msg), nil
}

func (lf *loggerFn) Close() error {
	return nil
}
