package issue

import (
	"bytes"
	"net/http"
	"strconv"
	"time"

	proto "github.com/golang/protobuf/proto"

	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/geolookup"
	"github.com/getlantern/flashlight/v7/logging"
	"github.com/getlantern/flashlight/v7/proxied"
	"github.com/getlantern/flashlight/v7/util"
	"github.com/getlantern/golog"
)

var (
	log        = golog.LoggerFor("flashlight.issue")
	maxLogSize = 10247680

	client = &http.Client{
		Transport: proxied.Fronted(0),
	}
)

const (
	requestURL = "https://iantem.io/api/v1/issue"
)

type Attachment struct {
	Name string
	Data []byte
}

// Sends an issue report to lantern-cloud/issue, which is then forwarded to ticket system via API
func SendReport(
	userConfig common.UserConfig,
	issueType int,
	description string,
	subscriptionLevel string,
	userEmail string,
	appVersion string,
	device string, // common name
	model string, // alphanumeric name
	osVersion string,
	attachments []*Attachment,
) (err error) {
	return sendReport(
		userConfig.GetDeviceID(),
		strconv.Itoa(int(userConfig.GetUserID())),
		userConfig.GetToken(),
		userConfig.GetLanguage(),
		issueType,
		description,
		subscriptionLevel,
		userEmail,
		appVersion,
		device,
		model,
		osVersion,
		attachments,
	)
}

func sendReport(
	deviceID string,
	userID string,
	proToken string,
	language string,
	issueType int,
	description string,
	subscriptionLevel string,
	userEmail string,
	appVersion string,
	device string,
	model string,
	osVersion string,
	attachments []*Attachment) error {
	r := &Request{}

	r.Type = Request_ISSUE_TYPE(issueType)
	r.CountryCode = geolookup.GetCountry(5 * time.Second)
	r.AppVersion = appVersion
	r.SubscriptionLevel = subscriptionLevel
	r.Platform = common.Platform
	r.Description = description
	r.UserEmail = userEmail
	r.DeviceId = deviceID
	r.UserId = userID
	r.ProToken = proToken
	r.Device = device
	r.Model = model
	r.OsVersion = osVersion
	r.Language = language

	for _, attachment := range attachments {
		r.Attachments = append(r.Attachments, &Request_Attachment{
			Type:    "application/zip",
			Name:    attachment.Name,
			Content: attachment.Data,
		})
	}

	// Zip logs
	if maxLogSize > 0 {
		if size, err := util.ParseFileSize(strconv.Itoa(maxLogSize)); err != nil {
			log.Error(err)
		} else {
			log.Debug("zipping log files for issue report")
			buf := &bytes.Buffer{}
			folder := "logs"
			if _, err := logging.ZipLogFiles(buf, folder, size, int64(maxLogSize)); err == nil {
				r.Attachments = append(r.Attachments, &Request_Attachment{
					Type:    "application/zip",
					Name:    folder + ".zip",
					Content: buf.Bytes(),
				})
			} else {
				log.Errorf("unable to zip log files: %v", err)
			}
		}
	}

	// send message to lantern-cloud
	out, err := proto.Marshal(r)
	if err != nil {
		log.Errorf("unable to marshal issue report: %v", err)
		return err
	}

	resp, err := client.Post(requestURL, "application/protobuf", bytes.NewBuffer(out))
	if err != nil {
		log.Errorf("unable to send issue report: %v", err)
		return err
	} else {
		log.Debugf("issue report sent: %v", resp)
	}

	return err
}
