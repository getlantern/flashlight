// Package email provides functionality for sending email messages via Mandrill
package email

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/keighl/mandrill"
	"go.uber.org/zap"

	"github.com/getlantern/errors"

	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/util"
)

var (
	log = zap.NewExample().Sugar()

	// Only allowed to call /send_template
	MandrillAPIKey = "fmYlUdjEpGGonI4NDx9xeA"

	defaultRecipient string
	mu               sync.RWMutex
)

func SetDefaultRecipient(address string) {
	mu.Lock()
	defer mu.Unlock()
	defaultRecipient = address
}

func getDefaultRecipient() string {
	mu.RLock()
	defer mu.RUnlock()
	return defaultRecipient
}

// Message is a templatized email message
type Message struct {
	// The mandrill template slug
	Template string
	// The email address of the sender
	From string
	// The email address to which the message is sent
	To string
	// An optional email address to carbon copy
	CC string `json:"cc,omitempty"`
	// Email subject
	Subject string
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
		op = ops.Begin("report_issue")
		if msg.Template == "user-send-logs-desktop" {
			// get parameters from desktop template vars
			isPro, _ := strconv.ParseBool(fmt.Sprint(msg.Vars["proUser"]))
			op.UserAgent(fmt.Sprint(msg.Vars["userAgent"])).
				Set("pro", isPro).
				Set("issue_type", msg.Vars["issueType"]).
				Set("issue_note", msg.Vars["note"]).
				Set("email", msg.Vars["email"])
		} else {
			// get parameters from mobile template vars
			isPro, _ := strconv.ParseBool(fmt.Sprint(msg.Vars["prouser"]))
			op.Set("pro", isPro).
				Set("issue_type", msg.Vars["issue"]).
				Set("issue_note", msg.Vars["report"]).
				Set("email", msg.Vars["emailaddress"])
		}
		log.Debug("Reporting issue")
	} else {
		op = ops.Begin("send_email").Set("template", msg.Template)
	}
	defer op.End()
	err := sendTemplate(msg)
	if err != nil {
		fail := op.FailIf(err)
		log.Error(fail)
		return fail
	}
	return nil
}

func sendTemplate(msg *Message) error {
	client := mandrill.ClientWithKey(MandrillAPIKey)
	recipient := msg.To
	if recipient == "" {
		recipient = getDefaultRecipient()
	}
	if recipient == "" {
		return errors.New("no recipient")
	}
	mmsg := &mandrill.Message{
		FromEmail: msg.From,
		To:        []*mandrill.To{&mandrill.To{Email: recipient}},
		Subject:   msg.Subject,
	}
	if msg.CC != "" {
		mmsg.To = append(mmsg.To, &mandrill.To{Email: msg.CC, Type: "cc"})
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
			if logging.ZipLogFiles(buf, folder, size) == nil {
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
