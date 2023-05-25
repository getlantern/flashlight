package ios

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/getlantern/flashlight/common"
	"github.com/stretchr/testify/assert"
)

var (
	appLogFileNames    = []string{"ios.log", "ios.log.1", "ios.log.2", "ios.log.3", "ios.log.4", "ios.log.5"}
	tunnelLogFileNames = []string{"lantern.log", "lantern.log.1", "lantern.log.2", "lantern.log.3", "lantern.log.4", "lantern.log.5"}
)

func TestReportIssue(t *testing.T) {
	appLogsDir, err := ioutil.TempDir("", "")
	if !assert.NoError(t, err) {
		return
	}
	defer os.RemoveAll(appLogsDir)
	tunnelLogsDir, err := ioutil.TempDir("", "")
	if !assert.NoError(t, err) {
		return
	}
	defer os.RemoveAll(tunnelLogsDir)
	for _, name := range appLogFileNames {
		if !assert.NoError(t, ioutil.WriteFile(filepath.Join(appLogsDir, name), []byte("I'm an app log!"), 0644)) {
			return
		}
	}
	for _, name := range tunnelLogFileNames {
		if !assert.NoError(t, ioutil.WriteFile(filepath.Join(tunnelLogsDir, name), []byte("I'm a tunnel log!"), 0644)) {
			return
		}
	}

	proxiesYaml, err := ioutil.TempFile("", "")
	if !assert.NoError(t, err) {
		return
	}
	defer proxiesYaml.Close()
	defer os.Remove(proxiesYaml.Name())
	_, err = proxiesYaml.Write([]byte("I'm a proxies.yaml!"))

	if !assert.NoError(t, err) {
		return
	}

	assert.NoError(t, reportIssueIos(
		common.UserConfig, // TODO why error?
		true,
		123,
		"iPhone 6",
		"iOS 9.3.2",
		"jay+unittest@getlantern.org",
		"this is a test issue",
		appLogsDir,
		tunnelLogsDir,
		proxiesYaml.Name()),
	)
}
