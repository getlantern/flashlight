package issue

import (
	"bytes"
	"encoding/base64"

	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/util"
	"github.com/getlantern/golog"
	"github.com/getlantern/mandrill"
)

var (
	log = golog.LoggerFor("issue")
)

const maxLogTailSize = 1024768

// Attachment represents a single supported attachment
type Attachment struct {
	// the MIME type of the attachment
	Type string `json:"type"`
	// the file name of the attachment
	Name string `json:"name"`
	// the content of the attachment as a base64-encoded string
	Content string `json:"content"`
}

type Report struct {
	Type              string
	CountryCode       string // ISO-3166-1
	AppVersion        string // *.*.*
	SubscriptionLevel string // free, pro, platnum
	Platform          string
	Description       string
	UserEmail         string
	attachments       []*Attachment
}

func (r *Report) Send(maxLogSize string) {
	if maxLogSize != "" {
		if size, err := util.ParseFileSize(maxLogSize); err != nil {
			log.Error(err)
		} else {
			log.Debug("Zipping log files for sending email")
			buf := &bytes.Buffer{}
			folder := "logs"
			if _, err := logging.ZipLogFiles(buf, folder, size, int64(maxLogTailSize)); err == nil {
				r.attachments = append(r.attachments, &mandrill.Attachment{
					Type:    "application/zip",
					Name:    folder + ".zip",
					Content: base64.StdEncoding.EncodeToString(buf.Bytes()),
				})
			} else {
				log.Errorf("Unable to zip log files: %v", err)
			}
		}
	}
}
