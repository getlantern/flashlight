package ios

import (
	"bytes"

	"github.com/getlantern/flashlight/email"
	"github.com/getlantern/flashlight/logging"
)

func init() {
	// TODO: get this from config
	email.SetDefaultRecipient("report-issue@getlantern.org")
}

// ReportIssue reports an issue via email.
func ReportIssue(appVersion string, deviceModel string, iosVersion string, emailAddress string, issue string, appLogsDir string, tunnelLogsDir string) error {
	msg := &email.Message{
		Template: "user-send-logs-ios",
		From:     emailAddress,
		Vars: map[string]interface{}{
			"issue":        issue,
			"emailaddress": emailAddress,
			"appversion":   appVersion,
			"iosmodel":     deviceModel,
			"iosversion":   iosVersion,
		},
	}
	b := &bytes.Buffer{}
	err := logging.ZipLogFilesFrom(b, 5*1024*1024, map[string]string{"app": appLogsDir, "tunnel": tunnelLogsDir})
	if err != nil {
		log.Errorf("Unable to zip log files: %v", err)
	} else {
		msg.Logs = b.Bytes()
	}
	return email.Send(msg)
}
