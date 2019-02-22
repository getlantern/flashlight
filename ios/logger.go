package ios

// #include <stdlib.h>
// #include <sys/types.h>
// static void callLogger(void *func, int level, const char *msg)
// {
// 	((void(*)(int, const char *))func)(level, msg);
// }
import "C"

import (
	"unsafe"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
)

// SetLogger configures the logger to use
func SetLogger(loggerFunc uint64) {
	lfn := unsafe.Pointer(uintptr(loggerFunc))
	golog.SetOutputs(&clogger{lfn, 1}, &clogger{lfn, 0})
}

// clogger implementation baesd on https://github.com/WireGuard/wireguard-ios
type clogger struct {
	loggerFunc unsafe.Pointer
	level      C.int
}

func (l *clogger) Write(p []byte) (int, error) {
	if uintptr(l.loggerFunc) == 0 {
		return 0, errors.New("No logger initialized")
	}
	message := C.CString(string(p))
	C.callLogger(l.loggerFunc, l.level, message)
	C.free(unsafe.Pointer(message))
	return len(p), nil
}
