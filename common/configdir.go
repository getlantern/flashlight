package common

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/getlantern/appdir"
)

// InConfigDir returns the path of the specified file name in the given
// configuration directory
func InConfigDir(configDir string, filename string) (string, error) {

	cdir := configDir
	if cdir == "" {
		cdir = appdir.General(AppName)
	}
	log.Debugf("Using config dir %v", cdir)
	if _, err := os.Stat(cdir); err != nil {
		if os.IsNotExist(err) {
			// Create config dir
			if err := os.MkdirAll(cdir, 0750); err != nil {
				return "", fmt.Errorf("Unable to create configdir at %s: %s", cdir, err)
			}
		}
	}

	return filepath.Join(cdir, filename), nil
}
