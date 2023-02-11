// package appdir provides a facility for determining the system-dependent
// paths for application resources.
package appdir

import (
	"fmt"
	"os/user"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
)

// General returns the path for general aplication resources (e.g.
// ~/Library/<App>).
func General(app string) string {
	return general(app)
}

// Logs returns the path for log files (e.g. ~/Library/Logs/<App>).
func Logs(app string) string {
	return logs(app)
}

func InHomeDir(filename string) string {
	usr, err := user.Current()
	if err == nil {
		return filepath.Join(usr.HomeDir, filename)
	}
	// "user: Current not implemented on ..." will happen on Linux or Darwin
	// when cross-compiled from other platforms.
	homeDir, err2 := homedir.Dir()
	if err2 != nil {
		panic(fmt.Errorf("Unable to determine user's home directory: %s, %s", err, err2))
	}
	return filepath.Join(homeDir, filename)
}
