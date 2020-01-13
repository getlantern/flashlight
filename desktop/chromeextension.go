package desktop

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/getlantern/golog"
)

const extensionID = "akppoapgnchinmnbinihafkogdohpbmk"

type extension struct {
	log golog.Logger
}

// chromeExtension allows callers to perform actions related to the Lantern
// Chrome extension.
type chromeExtension interface {
	// install installs the chrome extension using the external extension installation
	// method discussed at https://developer.chrome.com/extensions/external_extensions
	install()

	// installTo installs the chrome extension using the external extension installation
	// method discussed at https://developer.chrome.com/extensions/external_extensions
	// to the specified root extension directory. Useful for testing.
	installTo(string)

	// extensionDirs returns the OS-specific Chrome extension directories for our
	// extension. This can include multiple directories to account for multiple
	// Chrome profiles, for example, as well as for a local directory for extension
	// development.
	extensionDirs() ([]string, error)
}

// newChromeExtension creates a new chrome extension instance.
func newChromeExtension() chromeExtension {
	return &extension{
		log: golog.LoggerFor("chrome-extension"),
	}
}

func (e *extension) install() {
	// See https://developer.chrome.com/extensions/external_extensions for install
	// locations and procedures.
	switch runtime.GOOS {
	case "darwin":
		e.installDarwin()
	case "linux":
		e.installLinux()
	}
}

func (e *extension) installDarwin() {
	if base, err := e.osExtensionBasePath(runtime.GOOS); err != nil {
		e.log.Errorf("Could not get extension base path %v", err)
	} else {
		e.installTo(filepath.Join(base, "External Extensions"))
	}
}

func (e *extension) installLinux() {
	e.installTo(filepath.Join("usr", "share", "google-chrome", "extensions"))
}

func (e *extension) installTo(externalPath string) {
	if err := os.MkdirAll(externalPath, 0700); err != nil {
		e.log.Errorf("Could not make external extensions directory %v", err)
	} else {
		path := filepath.Join(externalPath, extensionID+".json")
		if f, err := os.Create(path); err != nil {
			e.log.Errorf("Could not open extension file for writing: %v", err)
		} else {
			if bytes, err := json.Marshal(map[string]string{
				"external_update_url": "https://clients2.google.com/service/update2/crx"}); err != nil {
				e.log.Errorf("Error marshaling map to JSON: %v", err)
			} else {
				if n, err := f.Write(bytes); err != nil {
					e.log.Errorf("Could not write extension %v", err)
				} else {
					e.log.Debugf("Saved extension to %s with size %v", path, n)
				}
			}
		}
	}
}

func (e *extension) extensionDirs() ([]string, error) {
	const fileName = "settings.json"
	paths := e.includeLocalExtension(fileName)
	if base, err := e.osExtensionBasePath(runtime.GOOS); err != nil {
		return paths, err
	} else {
		return e.extensionDirsForOS(extensionID, fileName, base, paths)
	}
}

func (e *extension) includeLocalExtension(fileName string) []string {
	// This allows us to use a local extension during development.
	if dir := os.Getenv("LANTERN_CHROME_EXTENSION"); dir != "" {
		return []string{filepath.Join(dir, fileName)}
	}
	return make([]string, 0)
}

func (e *extension) osExtensionBasePath(userOS string) (string, error) {
	if configdir, err := os.UserConfigDir(); err != nil {
		e.log.Errorf("Could not get config dir: %v", err)
		return "", err
	} else {
		switch userOS {
		case "windows":
			return filepath.Join(configdir, "..", "Local", "Google", "Chrome", "User Data"), nil
		case "darwin":
			return filepath.Join(configdir, "Google", "Chrome"), nil
		case "linux":
			base := filepath.Join(configdir, "google-chrome")
			if _, err := os.Stat(base); os.IsNotExist(err) {
				return filepath.Join(configdir, "chromium"), nil
			}
			return base, nil
		default:
			return "", fmt.Errorf("Unsupported operating system: %v", userOS)
		}
	}
}

// Gets the Chrome extension directories for our extension across operating systems.
func (e *extension) extensionDirsForOS(extensionID, fileName, base string, paths []string) ([]string, error) {
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return paths, err
	}

	// The user might have multiple profiles and/or multiple versions, so we just write to all
	// the relevant directories.
	if err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			e.log.Errorf("Could not walk extensions directory? %v", err)
			return err
		}
		if info.IsDir() && info.Name() == extensionID {
			if dirs, err := ioutil.ReadDir(path); err != nil {
				e.log.Errorf("Could not read extension folders %v", err)
				return err
			} else {
				// This directory can include subdirectories for multiple versions of the extension.
				// Just include the paths of all versions for simplicity and write to all of them.
				for _, fi := range dirs {
					if fi.IsDir() {
						paths = append(paths, filepath.Join(path, fi.Name(), "data", fileName))
					}
				}
			}
			return nil
		}
		return nil
	}); err != nil {
		e.log.Errorf("Error walking extensions directory")
		return paths, err
	}
	e.log.Debugf("Returning Chrome extension paths: %#v", paths)
	return paths, nil
}
