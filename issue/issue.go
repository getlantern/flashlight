package issue

import (
	"bytes"
	"net/http"
	"strconv"
	"time"

	proto "github.com/golang/protobuf/proto"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/flashlight/util"
	"github.com/getlantern/golog"
)

var (
	log            = golog.LoggerFor("flashlight.issue")
	maxLogTailSize = 1024768   // TODO are both of these needed?
	maxLogSize     = "1024768" // TODO are both of these needed?

	client = &http.Client{
		Transport: proxied.ChainedThenFronted(),
	}
)

const (
	requestURL = "https://issue.lantr.net/issue"
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
	device string,
	model string,
	osVersion string,
	attachments []*Attachment,
) (err error) {

	r := &Request{}

	log.Debug("capturing issue report metadata")
	r.Type = Request_ISSUE_TYPE(issueType)
	r.CountryCode = geolookup.GetCountry(5 * time.Second)
	r.AppVersion = common.Version
	r.SubscriptionLevel = subscriptionLevel
	r.Platform = common.Platform
	r.Description = description
	r.UserEmail = userEmail
	r.DeviceId = userConfig.GetDeviceID()
	r.UserId = strconv.Itoa(int(userConfig.GetUserID()))
	r.ProToken = userConfig.GetToken()
	r.Device = device
	r.Model = model
	r.OsVersion = osVersion
	r.Language = userConfig.GetLanguage()

	for _, attachment := range attachments {
		r.Attachments = append(r.Attachments, &Request_Attachment{
			Type:    "application/zip",
			Name:    attachment.Name,
			Content: attachment.Data,
		})
	}

	// Zip logs
	if maxLogSize != "" {
		if size, err := util.ParseFileSize(maxLogSize); err != nil {
			log.Error(err)
		} else {
			log.Debug("zipping log files for issue report")
			buf := &bytes.Buffer{}
			folder := "logs"
			if _, err := logging.ZipLogFiles(buf, folder, size, int64(maxLogTailSize)); err == nil {
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

	resp, err := client.Post(requestURL, "application/zip", bytes.NewBuffer(out))
	if err != nil {
		log.Errorf("unable to send issue report: %v", err)
		return err
	} else {
		log.Debugf("issue report sent: %v", resp)
	}

	return err
}
