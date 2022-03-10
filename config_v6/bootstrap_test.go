package config

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/getlantern/yaml"
	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/common"
)

func TestBootstrapSettings(t *testing.T) {
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
	settings := BootstrapSettings{StartupUrl: "test"}
	log.Debugf("Settings: %v", settings)

	data, er := yaml.Marshal(&settings)
	assert.True(t, er == nil, "Should not be an error")

	e := ioutil.WriteFile(file.Name(), data, 0644)
	assert.True(t, e == nil, "Should not be an error")

	ps, errr := readSettingsFromFile(file.Name())
	assert.Equal(t, "test", ps.StartupUrl, "Unexpected startup URL")
	assert.NoError(t, errr, "Unable to read settings")

	_, path, err := bootstrapPath(name)
	assert.True(t, err == nil, "Should not be an error")

	var dir string

	if common.Platform == "darwin" {
		dir, err = filepath.Abs(filepath.Dir(os.Args[0]) + "/../Resources")
	} else if common.Platform == "linux" {
		dir, err = filepath.Abs(filepath.Dir(os.Args[0]) + "/../")
	}
	assert.True(t, err == nil, "Should not be an error")

	log.Debugf("Running in %v", dir)
	if common.Platform == "darwin" {
		assert.Equal(t, dir+"/"+name, path, "Unexpected settings dir")
	} else if common.Platform == "linux" {
		assert.Equal(t, dir+"/"+name, path, "Unexpected settings dir")
	}
}
