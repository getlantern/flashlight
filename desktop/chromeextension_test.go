package desktop

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChromeExtension(t *testing.T) {
	e := newChromeExtension().(*extension)

	basePath, err := e.osExtensionBasePath("windows")
	assert.NoError(t, err)
	assert.True(t, strings.Contains(basePath, "Local"))

	basePath, err = e.osExtensionBasePath("darwin")
	assert.NoError(t, err)
	assert.True(t, strings.Contains(basePath, "Google"))

	basePath, err = e.osExtensionBasePath("linux")
	assert.NoError(t, err)
	assert.True(t, strings.Contains(basePath, "chromium"))

	_, err = e.osExtensionBasePath("eurequrq9ur")
	assert.Error(t, err)

	dirs, err := e.extensionDirsForOS("doesnotexist", "settings.json", "nodirectoryhere", make([]string, 0))
	assert.Error(t, err)
	assert.Equal(t, 0, len(dirs))

	dir, err := ioutil.TempDir("", "testconfig")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	dirs, err = e.extensionDirsForOS("doesnotexist", "settings.json", dir, make([]string, 0))
	assert.NoError(t, err)
	assert.Equal(t, 0, len(dirs))

	dirs, err = e.extensionDirsForOS("doesnotexist", "settings.json", dir, make([]string, 0))
	assert.NoError(t, err)
	assert.Equal(t, 0, len(dirs))

	// Create a dummy extension directory under our temp config path.
	err = os.MkdirAll(filepath.Join(dir, "direxists", "0.0.1"), 0700)
	assert.NoError(t, err)

	dirs, err = e.extensionDirsForOS("direxists", "settings.json", dir, make([]string, 0))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(dirs))

	e.installTo(dir)
	external := filepath.Join(dir, extensionID+".json")
	content, err := ioutil.ReadFile(external)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(content), "https://clients2.google.com/service/update2/crx"))
}
