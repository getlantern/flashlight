//go:build integration
// +build integration

package issue

import (
	"os"
	"testing"

	"github.com/getlantern/golog"
)

var logger = golog.LoggerFor("issue_test")

func TestMain(m *testing.M) {
	client = &http.Client{}
	tempConfigDir, err := os.MkdirTemp("", "issue_test")
	if err != nil {
		logger.Errorf("Unable to create temp config dir: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempConfigDir)
	os.Exit(m.Run())
}

func TestSendIssueReport(t *testing.T) {
	err := SendIssueReport("test", "US", "1.0.0", "free", "ios", "test", "jay+test@getlantern.org", [][]byte{})
	if err != nil {
		t.Errorf("SendIssueReport() error = %v", err)
	}
}
