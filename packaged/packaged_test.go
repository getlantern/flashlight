package packaged

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getlantern/yaml"
	"github.com/stretchr/testify/assert"
)

func TestPackagedSettings(t *testing.T) {
	file, err := ioutil.TempFile("", ".packaged-lantern.yaml")
	defer func() {
		err := os.Remove(file.Name())
		if err != nil {
			log.Errorf("Could not remove file? %v", err)
		}
	}()
	assert.True(t, err == nil, "Should not be an error")
	file.Close()

	log.Debugf("File at: %v", file.Name())
	settings := PackagedSettings{StartupUrl: "test"}
	log.Debugf("Settings: %v", settings)

	data, er := yaml.Marshal(&settings)
	assert.True(t, er == nil, "Should not be an error")

	e := ioutil.WriteFile(file.Name(), data, 0644)
	assert.True(t, e == nil, "Should not be an error")

	path, ps, errr := readSettingsFromFile(file.Name())
	assert.Equal(t, "test", ps.StartupUrl, "Unexpected startup URL")
	assert.Equal(t, file.Name(), path, "Wrote to unexpected path")
	assert.True(t, errr == nil, "Should not be an error")

	// Now do another full round trip, writing and reading
	// Overwite local to avoid affecting actual Lantern instances
	local = file.Name()
	path, errrr := writeToDisk(ps)
	assert.True(t, errrr == nil, "Should not be an error")
	path, ps, err = readSettingsFromFile(path)
	assert.Equal(t, "test", ps.StartupUrl, "Could not read data")
	assert.Equal(t, local, path, "Wrote to unexpected path")
	assert.True(t, err == nil, "Should not be an error")

	url = "https://testing.com"

	path, ps, err = readSettingsFromFile(path)

	log.Debugf("Wrote settings to: %v", path)
	assert.Equal(t, url, ps.StartupUrl, "Could not read data")
	assert.Equal(t, local, path, "Wrote to unexpected path")
	assert.True(t, err == nil, "Should not be an error")

	path, err = packagedSettingsPath()
	assert.True(t, err == nil, "Should not be an error")

	var dir string
	dir, err = filepath.Abs(filepath.Dir(os.Args[0]))
	assert.True(t, err == nil, "Should not be an error")

	if runtime.GOOS == "darwin" {
		assert.Equal(t, dir+"/../Resources/"+name, path, "Unexpected settings dir")
	} else if runtime.GOOS == "linux" {
		assert.Equal(t, dir+"/"+name, path, "Unexpected settings dir")
	}
}
