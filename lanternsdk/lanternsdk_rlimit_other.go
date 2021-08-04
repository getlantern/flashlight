// +build !ios

package lanternsdk

import (
	"syscall"
)

const (
	highFileLimit = 16384
)

// on iOS, the default file descriptor limit for the process is too low to accommodate Lantern, bump it up
func increaseFilesLimit() {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Errorf("unable to get current RLIMIT_NOFILE: %v", err)
		return
	}
	rLimit.Cur = highFileLimit
	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Errorf("unable to set RLIMIT_NOFILE: %v", err)
	}
}
