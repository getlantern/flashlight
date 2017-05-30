// Package email provides functionality for sending email messages via Mandrill
package email

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/keighl/mandrill"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/osversion"

	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/util"
)

var (
	log = golog.LoggerFor("flashlight.email")

	// Only allowed to call /send_template
	MandrillAPIKey = "fmYlUdjEpGGonI4NDx9xeA"
)

// Message is a templatized email message
type Message struct {
	// The mandrill template slug
	Template string
	// The email address to which the message is sent
	To string
	// Any global vars defined in the template
	Vars map[string]interface{}
	// Serialized settings data
	SettingsData []byte
	// Specify the maximum size of the not-compressed log files attached to the
	// email, can be in "KB/MB/GB" fromat. Not attaching logs if it's an empty
	// string.  Make sure the compressed files doesn't cause the message to exceed
	// the limit of Mandrill message size, which is 25MB.
	MaxLogSize string `json:"maxLogSize,omitempty"`
}

func Send(msg *Message) error {
	var op *ops.Op
	if strings.HasPrefix(msg.Template, "user-send-logs") {
		isPro, _ := strconv.ParseBool(fmt.Sprint(msg.Vars["proUser"]))
		op = ops.Begin("report_issue").
			UserAgent(fmt.Sprint(msg.Vars["userAgent"])).
			Set("pro", isPro).
			Set("issue_type", msg.Vars["issueType"]).
			Set("issue_note", msg.Vars["note"]).
			Set("email", msg.Vars["email"])
		log.Debug("Reporting issue")
	} else {
		op = ops.Begin("send_email").Set("template", msg.Template)
	}
	defer op.End()
	fillDefaults(msg)
	err := sendTemplate(msg)
	if err != nil {
		return log.Error(op.FailIf(err))
	}
	return nil
}

func sendTemplate(msg *Message) error {
	client := mandrill.ClientWithKey(MandrillAPIKey)
	mmsg := &mandrill.Message{
		To: []*mandrill.To{&mandrill.To{Email: msg.To}},
	}
	mmsg.GlobalMergeVars = mandrill.MapToVars(msg.Vars)
	if msg.SettingsData != nil {
		mmsg.Attachments = append(mmsg.Attachments, &mandrill.Attachment{
			Type:    "application/x-yaml",
			Name:    prefix(msg) + "_settings.yaml",
			Content: base64.StdEncoding.EncodeToString(msg.SettingsData),
		})
	}
	if msg.MaxLogSize != "" {
		if size, err := util.ParseFileSize(msg.MaxLogSize); err != nil {
			log.Error(err)
		} else {
			buf := &bytes.Buffer{}
			folder := prefix(msg) + "_logs"
			if logging.ZipLogFiles(buf, "", folder, size) == nil {
				mmsg.Attachments = append(mmsg.Attachments, &mandrill.Attachment{
					Type:    "application/zip",
					Name:    folder + ".zip",
					Content: base64.StdEncoding.EncodeToString(buf.Bytes()),
				})
			}
		}
	}
	responses, err := client.MessagesSendTemplate(mmsg, msg.Template, "")
	if err != nil {
		return err
	}

	return readResponses(responses)
}

func readResponses(responses []*mandrill.Response) error {
	// There's exactly one response, use "for" loop for simpler code.
	for _, resp := range responses {
		switch resp.Status {
		case "sent", "queued", "scheduled":
			return nil
		case "rejected":
			return errors.New("rejected: " + resp.RejectionReason)
		default:
			return errors.New(resp.Status)
		}
	}
	return nil
}

func fillDefaults(msg *Message) {
	if msg.Vars == nil {
		// avoid panicking in case the message is malformed
		msg.Vars = make(map[string]interface{})
	}
	os, err := osversion.GetHumanReadable()
	if err != nil {
		log.Errorf("Unable to get version: %v", err)
	} else {
		msg.Vars["os"] = os
	}
}

func prefix(msg *Message) string {
	prepend := func(v interface{}, ts string) string {
		if s, ok := v.(string); ok && s != "" {
			return s + "_" + ts
		}
		return ts
	}
	s := time.Now().Format("060102T15:04MST")
	s = prepend(msg.Vars["userID"], s)
	s = prepend(msg.Vars["email"], s)
	s = prepend(msg.Vars["os"], s)
	s = prepend(msg.Vars["version"], s)
	return s
}
