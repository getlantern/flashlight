package ios

import (
	"bytes"
	"net/http"
	"time"

	"github.com/getlantern/flashlight/email"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/fronted"
)

func init() {
	email.SetDefaultRecipient("getlantern@inbox.groovehq.com")

	go func() {
		log.Debug("Getting fronted transport to use for submitting issues")
		start := time.Now()
		tr, ok := fronted.NewDirect(5 * time.Minute)
		if ok {
			log.Debugf("Got fronted transport for submitting issues within %v", time.Now().Sub(start))
		} else {
			log.Debug("Failed to get fronted transport for submitting issues")
		}
		email.SetHTTPClient(&http.Client{
			Timeout:   20 * time.Second,
			Transport: tr,
		})
	}()
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
	// 5MB is logfile size limit, and we have:
	// two targets (app/netEx) with their own logs
	// each target has 6 ios log files and 6 go log files
	// for a total of 24 files * 5MB 
	err := logging.ZipLogFilesFrom(b, 5*1024*1024*24, map[string]string{"app": appLogsDir, "tunnel": tunnelLogsDir})
	if err != nil {
		log.Errorf("Unable to zip log files: %v", err)
	} else {
		msg.Logs = b.Bytes()
	}
	return email.Send(msg)
}
