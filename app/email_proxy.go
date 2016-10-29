package app

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/keighl/mandrill"

	"github.com/getlantern/errors"
	"github.com/getlantern/osversion"

	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/ui"
	"github.com/getlantern/flashlight/util"
)

type mandrillMessage struct {
	// The mandrill template slug
	Template string
	// The email address to which the message is sent
	To string
	// Any global vars defined in the template
	Vars map[string]interface{}
	// If attach the settings file to the email or not
	WithSettings bool `json:"withSettings,omitempty"`
	// Specify the maximum size of the log files attached to the email, can be
	// in "KB/MB/GB" fromat. Not attaching logs if it's an empty string.
	MaxLogSize string `json:"maxLogSize,omitempty"`
}

var (
	// Only allowed to call /send_template
	mandrillAPIKey = "fmYlUdjEpGGonI4NDx9xeA"
)

// A proxy that accept requests from WebSocket and send email via 3rd party
// service (mandrill atm). With optionally attached settings and Lantern logs.
// It intentionally uses direct connection to the 3rd party service, to serve
// as an out-of-band channel when Lantern doesn't work well, say, when user
// wants to report an issue.
func serveEmailProxy() error {
	service, err := ui.RegisterWithMsgInitializer("email-proxy", nil,
		func() interface{} { return &mandrillMessage{} })
	if err != nil {
		log.Errorf("Error registering with UI? %v", err)
		return err
	}
	go read(service)
	return nil
}

func read(service *ui.Service) {
	for message := range service.In {
		data, ok := message.(*mandrillMessage)
		if !ok {
			log.Errorf("Malformatted message %v", message)
			continue
		}
		fillDefaults(data)
		if err := sendTemplate(data); err != nil {
			log.Error(err)
			service.Out <- err.Error()
		} else {
			service.Out <- "success"
		}
	}
}

func sendTemplate(data *mandrillMessage) error {
	client := mandrill.ClientWithKey(mandrillAPIKey)
	msg := &mandrill.Message{
		To: []*mandrill.To{&mandrill.To{Email: data.To}},
	}
	msg.GlobalMergeVars = mandrill.MapToVars(data.Vars)
	var buf bytes.Buffer
	if data.WithSettings {
		if _, err := settings.writeTo(&buf); err != nil {
			log.Error(err)
		} else {
			msg.Attachments = append(msg.Attachments, &mandrill.Attachment{
				Type:    "application/x-yaml",
				Name:    prefix(data) + "_settings.yaml",
				Content: base64.StdEncoding.EncodeToString(buf.Bytes()),
			})
		}
	}
	if data.MaxLogSize != "" {
		if size, err := util.ParseFileSize(data.MaxLogSize); err != nil {
			log.Error(err)
		} else {
			buf.Reset()
			folder := prefix(data) + "_logs"
			if logging.ZipLogFiles(&buf, "", folder, size) == nil {
				msg.Attachments = append(msg.Attachments, &mandrill.Attachment{
					Type:    "application/zip",
					Name:    folder + ".zip",
					Content: base64.StdEncoding.EncodeToString(buf.Bytes()),
				})
			}
		}
	}
	responses, err := client.MessagesSendTemplate(msg, data.Template, "")
	if err != nil {
		return err
	}
	// There's exactly one response, use "for" loop for simpler code.
	for _, resp := range responses {
		switch resp.Status {
		case "sent", "queued":
			return nil
		case "rejected":
			return errors.New("rejected: " + resp.RejectionReason)
		default:
			return errors.New(resp.Status)
		}
	}
	return nil
}

func fillDefaults(msg *mandrillMessage) {
	msg.Vars["userID"] = settings.GetUserID()
	msg.Vars["deviceID"] = settings.GetDeviceID()
	msg.Vars["proToken"] = settings.GetToken()
	os, err := osversion.GetHumanReadable()
	if err != nil {
		log.Error(err)
	} else {
		msg.Vars["os"] = os
	}
	msg.Vars["version"] = fmt.Sprintf("%v (%v)",
		settings.getString(SNVersion),
		settings.getString(SNRevisionDate))
}

func prefix(msg *mandrillMessage) string {
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
