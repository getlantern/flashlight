package common

import (
	"fmt"
	"os"
	"path/filepath"
)

// InConfigDir returns the path of the specified file name in the given
// configuration directory
func InConfigDir(configDir string, filename string) (string, error) {
	log.Debugf("Using config dir %v", configDir)
	if _, err := os.Stat(configDir); err != nil {
		if os.IsNotExist(err) {
			// Create config dir
			if err := os.MkdirAll(configDir, 0750); err != nil {
				return "", fmt.Errorf("Unable to create configdir at %s: %s", configDir, err)
			}
		}
	}

	return filepath.Join(configDir, filename), nil
}
