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

func loggerWith(fn func(string)) io.Writer {
	return &loggerFn{fn}
}

type loggerFn struct {
	log func(string)
}

func (lf *loggerFn) Write(msg []byte) (int, error) {
	lf.log(string(msg))
	return len(msg), nil
}
