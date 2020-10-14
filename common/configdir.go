package common

import (
	"path/filepath"
)

// InConfigDir returns the path of the specified file name in the given
// configuration directory
func InConfigDir(configDir string, filename string) string {
	return filepath.Join(configDir, filename)
}
