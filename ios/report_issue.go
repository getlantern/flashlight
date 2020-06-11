package ios

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/getlantern/flashlight/email"
	"github.com/getlantern/flashlight/flfronting"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/fronted"
)

func init() {
	email.SetDefaultRecipient("getlantern@inbox.groovehq.com")

	go func() {
		log.Debug("Getting fronted transport to use for submitting issues")

		ctx, cancel := context.WithTimeout(context.Background(), frontedAvailableTimeout)
		defer cancel()

		start := time.Now()
		tr, err := flfronting.NewRoundTripper(ctx, fronted.RoundTripperOptions{})
		if err != nil {
			log.Debugf("Failed to obtain fronted transport for submitting issue: %w", err)
		} else {
			log.Debugf("Got fronted transport for submitting issues within %v", time.Since(start))
		}
		email.SetHTTPClient(&http.Client{
			Timeout: 20 * time.Second,
			// If we failed to get a fronted transport, this will default to http.DefaultTransport.
			Transport: tr,
		})
	}()
}

// ReportIssue reports an issue via email.
func ReportIssue(isPro bool, userID int, proToken, deviceID, appVersion, deviceModel, iosVersion, emailAddress, issue, appLogsDir, tunnelLogsDir, proxiesYamlPath string) error {
	proText := "no"
	if isPro {
		proText = "yes"
	}
	msg := &email.Message{
		Template: "user-send-logs-ios",
		From:     emailAddress,
		Vars: map[string]interface{}{
			"issue":        issue,
			"userid":       userID,
			"protoken":     proToken,
			"prouser":      proText,
			"deviceID":     deviceID,
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

	bytes, err := ioutil.ReadFile(proxiesYamlPath)
	if err != nil {
		log.Errorf("Unable to read proxies.yaml for reporting issue: %v", err)
	} else {
		msg.Proxies = bytes
	}

	return email.Send(msg)
}
