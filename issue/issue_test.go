//go:build integration
// +build integration

package issue

import (
	"crypto/tls"
	"net/http"
	"os"
	"testing"

	"github.com/getlantern/golog"
)

var logger = golog.LoggerFor("issue_test")

func TestMain(m *testing.M) {
	client = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				// TODO: Move the service to iantem.io
				InsecureSkipVerify: true,
			},
		},
	}
	tempConfigDir, err := os.MkdirTemp("", "issue_test")
	if err != nil {
		logger.Errorf("Unable to create temp config dir: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempConfigDir)
	os.Exit(m.Run())
}

func TestSendReport(t *testing.T) {
	err := sendReport(
		"34qsdf-24qsadf-32542q",
		"1",
		"token",
		"en",
		int(Request_NO_ACCESSS),
		"Description placeholder",
		"pro",
		"jay+test@getlantern.org",
		"Samsung Galaxy S10",
		"SM-G973F",
		"11",
		[]*Attachment{
			{
				Name: "Hello.txt",
				Data: []byte("Hello World"),
			},
		})
	if err != nil {
		t.Errorf("SendReport() error = %v", err)
	}
}
