package app

import (
	"bytes"
	"encoding/base64"
	"time"

	"github.com/keighl/mandrill"

	"github.com/getlantern/errors"

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
	WithSettings bool `json:"withSettings",omitempty`
	// Specify the maximum size of the log files attached to the email, can be
	// in "KB/MB/GB" fromat. Not attaching logs if it's zero or an empty string.
	MaxLogSize string `json:"maxLogSize",omitempty`
}

var (
	// Only allowed to call /send_template
	mandrillAPIKey = "fmYlUdjEpGGonI4NDx9xeA"
)

func serveMandrill() error {
	service, err := ui.RegisterWithMsgInitializer("mandrill", nil,
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

		if err := send(data); err != nil {
			service.Out <- err.Error()
		}
		service.Out <- "success"
	}
}

func send(data *mandrillMessage) error {
	client := mandrill.ClientWithKey(mandrillAPIKey)
	msg := &mandrill.Message{
		To: []*mandrill.To{&mandrill.To{Email: data.To}},
	}
	msg.GlobalMergeVars = mandrill.MapToVars(data.Vars)
	var buf bytes.Buffer
	if data.WithSettings {
		if _, err := settings.writeTo(&buf); err == nil {
			msg.Attachments = append(msg.Attachments, &mandrill.Attachment{
				Type:    "application/x-yaml",
				Name:    prefix(data) + "_settings.yaml",
				Content: base64.StdEncoding.EncodeToString(buf.Bytes()),
			})
		}
	}
	if data.MaxLogSize != "" {
		if size, err := util.ParseFileSize(data.MaxLogSize); err == nil {
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
