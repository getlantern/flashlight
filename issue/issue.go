package issue

import (
	"github.com/getlantern/golog"
)

var (
	log            = golog.LoggerFor("flashlight.issue")
	maxLogTailSize = 1024768
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

func SendIssueReport(
	issueType string,
	countryCode string,
	appVersion string,
	subscriptionLevel string,
	platform string,
	description string,
	userEmail string,
	attachments [][]byte,
) (err error) {

	r := &Request{}

	log.Debug("sending issue report")
	r.Type = issueType
	r.CountryCode = countryCode
	r.AppVersion = appVersion
	r.SubscriptionLevel = subscriptionLevel
	r.Platform = platform
	r.Description = description
	r.UserEmail = userEmail

	// log.Debug("zipping attachments for issue report")
	// for _, attachment := range attachments {
	// 	r.Attachments = append(r.Attachments, &Request_Attachment{
	// 		Type:    "application/zip",
	// 		Name:    "attachment",
	// 		Content: attachment,
	// 	})
	// }

	// // Zip attachments
	// if maxLogSize != "" {
	// 	if size, err := util.ParseFileSize(maxLogSize); err != nil {
	// 		log.Error(err)
	// 	} else {
	// 		log.Debug("Zipping log files for sending email")
	// 		buf := &bytes.Buffer{}
	// 		folder := "logs"
	// 		// attachmentfoo := make([]*issue.Attachment, 0)
	// 		if _, err := logging.ZipLogFiles(buf, folder, size, int64(maxLogTailSize)); err == nil {
	// 			r.Attachments = append(r.Attachments, &Request_Attachment{
	// 				Type:    "application/zip",
	// 				Name:    folder + ".zip",
	// 				Content: buf.Bytes(),
	// 			})
	// 		} else {
	// 			log.Errorf("Unable to zip log files: %v", err)
	// 		}
	// 	}
	// }

	// TODO send message to lantern-cloud
	return nil
}
