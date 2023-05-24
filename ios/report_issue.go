package ios

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/getlantern/flashlight/email"
	"github.com/getlantern/flashlight/issue"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/fronted"
)

func init() {
	// email.SetDefaultRecipient("support@lantern.jitbit.com")

	go func() {
		log.Debug("Getting fronted transport to use for submitting issues")
		start := time.Now()
		tr, ok := fronted.NewDirect(longFrontedAvailableTimeout)
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

// // Deprecated in favor of reportIssueViaAPI()
// // ReportIssueViaEmail reports an issue via email.
// func ReportIssueViaEmail(isPro bool, userID int, proToken, deviceID, appVersion, deviceModel, iosVersion, emailAddress, issue, appLogsDir, tunnelLogsDir, proxiesYamlPath string) error {
// 	proText := "no"
// 	if isPro {
// 		proText = "yes"
// 	}
// 	msg := &email.Message{
// 		Template: "user-send-logs-ios",
// 		From:     emailAddress,
// 		Vars: map[string]interface{}{
// 			"issue":        issue,
// 			"userid":       userID,
// 			"protoken":     proToken,
// 			"prouser":      proText,
// 			"deviceID":     deviceID,
// 			"emailaddress": emailAddress,
// 			"appversion":   appVersion,
// 			"iosmodel":     deviceModel,
// 			"iosversion":   iosVersion,
// 		},
// 	}
// 	b := &bytes.Buffer{}
// 	// 5MB is logfile size limit, and we have:
// 	// two targets (app/netEx) with their own logs
// 	// each target has 6 ios log files and 6 go log files
// 	// for a total of 24 files * 5MB
// 	_, err := logging.ZipLogFilesFrom(b, 5*1024*1024*24, 0, map[string]string{"app": appLogsDir, "tunnel": tunnelLogsDir})
// 	if err != nil {
// 		log.Errorf("Unable to zip log files: %v", err)
// 	} else {
// 		msg.Logs = b.Bytes()
// 	}

// 	bytes, err := ioutil.ReadFile(proxiesYamlPath)
// 	if err != nil {
// 		log.Errorf("Unable to read proxies.yaml for reporting issue: %v", err)
// 	} else {
// 		msg.Proxies = bytes
// 	}

// 	return email.Send(context.TODO(), msg)
// }

// reportIssueViaAPI reports an issue via the lantern-cloud/issue API
// TODO where should this be invoked?
func reportIssueViaAPI(
	isPro bool,
	userID int,
	proToken,
	deviceID,
	appVersion,
	deviceModel,
	iosVersion,
	emailAddress,
	issueText,
	appLogsDir,
	tunnelLogsDir,
	proxiesYamlPath string) (err error) {

	proText := "no"
	if isPro {
		proText = "yes"
	}

	description := deviceID // TODO determine where the "description" field is passed in from iOS
	platform := fmt.Sprintf("%v (iOS %v)", deviceModel, iosVersion)

	// make an empty slice of a slice of bytes
	attachments := [][]byte{}

	// attach app logs and tunnel logs
	b := &bytes.Buffer{}
	// 5MB is logfile size limit, and we have:
	// two targets (app/netEx) with their own logs
	// each target has 6 ios log files and 6 go log files
	// for a total of 24 files * 5MB
	_, err = logging.ZipLogFilesFrom(b, 5*1024*1024*24, 0, map[string]string{"app": appLogsDir, "tunnel": tunnelLogsDir})
	if err != nil {
		log.Errorf("unable to zip log files: %v", err)
	} else {
		attachments = append(attachments, b.Bytes())
	}

	// attach proxies.yaml
	bytes, err := ioutil.ReadFile(proxiesYamlPath)
	if err != nil {
		log.Errorf("unable to read proxies.yaml for reporting issue: %v", err)
	} else {
		attachments = append(attachments, bytes)
	}

	// TODO determine the country code
	err = issue.SendReport(issueText, "country-code-placeholder", appVersion, proText, platform, description, emailAddress, attachments)
	if err != nil {
		log.Errorf("unable to send ios issue report: %v", err)
	}

	return err
}
