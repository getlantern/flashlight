package issue

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/getlantern/golog"
)

var logger = golog.LoggerFor("issue-test")

func TestMain(m *testing.M) {
	tempConfigDir, err := ioutil.TempDir("", "issue_test")
	if err != nil {
		logger.Errorf("Unable to create temp config dir: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempConfigDir)
	os.Exit(m.Run())
}
