package issue

import (
	"bytes"
	"fmt"
	"net/http"

	proto "github.com/golang/protobuf/proto"

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

// Attachment represents a single supported attachment
// type Attachment struct {
// 	// the MIME type of the attachment
// 	Type string `json:"type"`
// 	// the file name of the attachment
// 	Name string `json:"name"`
// 	// the content of the attachment as a base64-encoded string
// 	Content string `json:"content"`
// }

// type Report struct {
// 	Type              string
// 	CountryCode       string // ISO-3166-1
// 	AppVersion        string // *.*.*
// 	SubscriptionLevel string // free, pro, platnum
// 	Platform          string
// 	Description       string
// 	UserEmail         string
// 	attachments       []*Attachment
// }

// Sends an issue report to lantern-cloud/issue, which is then forwarded to ticket system via API
func SendIssueReport(
	issueType int32,
	countryCode string,
	appVersion string,
	subscriptionLevel string,
	platform string,
	description string,
	userEmail string,
	attachments [][]byte,
) (err error) {

	r := &Request{}

	log.Debug("capturing issue report metadata")
	r.Type = Request_ISSUE_TYPE(issueType)
	r.CountryCode = countryCode
	r.AppVersion = appVersion
	r.SubscriptionLevel = subscriptionLevel
	r.Platform = platform
	r.Description = description
	r.UserEmail = userEmail

	for _, attachment := range attachments {
		r.Attachments = append(r.Attachments, &Request_Attachment{
			Type:    "application/zip",
			Name:    "attachment",
			Content: attachment,
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
	port := 443
	destination := "https://issue.lantr.net/issue" // TODO verify desitination and port
	requestURL := fmt.Sprintf("%v:%v", destination, port)
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
