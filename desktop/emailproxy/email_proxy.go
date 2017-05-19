package emailproxy

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
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
	"github.com/getlantern/flashlight/ws"
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
	// Specify the maximum size of the not-compressed log files attached to the
	// email, can be in "KB/MB/GB" fromat. Not attaching logs if it's an empty
	// string.  Make sure the compressed files doesn't cause the message to exceed
	// the limit of Mandrill message size, which is 25MB.
	MaxLogSize string `json:"maxLogSize,omitempty"`
}

var (
	log = golog.LoggerFor("flashlight.desktop.emailproxy")

	// Only allowed to call /send_template
	mandrillAPIKey = "fmYlUdjEpGGonI4NDx9xeA"

	defaultVars map[string]interface{}
	st          Settings
)

type Settings interface {
	GetUserID() int64
	GetToken() string
	WriteTo(w io.Writer) (int, error)
}

// A proxy that accept requests from WebSocket and send email via 3rd party
// service (mandrill atm). With optionally attached settings and Lantern logs.
// It intentionally uses direct connection to the 3rd party service, to serve
// as an out-of-band channel when Lantern doesn't work well, say, when user
// wants to report an issue.
func Start(s Settings, deviceID string, version string, revisionDate string) error {
	st = s
	defaultVars = defaults(deviceID, version, revisionDate)
	service, err := ws.RegisterWithMsgInitializer("email-proxy", nil,
		func() interface{} { return &mandrillMessage{} })
	if err != nil {
		log.Errorf("Error registering with UI? %v", err)
		return err
	}
	go read(service)
	return nil
}

func defaults(deviceID string, version string, revisionDate string) map[string]interface{} {
	vars := make(map[string]interface{})
	vars["deviceID"] = deviceID
	os, err := osversion.GetHumanReadable()
	if err != nil {
		log.Error(err)
	} else {
		vars["os"] = os
	}
	vars["version"] = fmt.Sprintf("%v (%v)", version, revisionDate)
	return vars
}

func read(service *ws.Service) {
	for message := range service.In {
		data, ok := message.(*mandrillMessage)
		if !ok {
			log.Errorf("Malformatted message %v", message)
			continue
		}
		handleMessage(service, data)
	}
}

func handleMessage(service *ws.Service, data *mandrillMessage) {
	var op *ops.Op
	if strings.HasPrefix(data.Template, "user-send-logs") {
		isPro, _ := strconv.ParseBool(fmt.Sprint(data.Vars["proUser"]))
		op = ops.Begin("report_issue").
			UserAgent(fmt.Sprint(data.Vars["userAgent"])).
			Set("pro", isPro).
			Set("issue_type", data.Vars["issueType"]).
			Set("issue_note", data.Vars["note"]).
			Set("email", data.Vars["email"])
		log.Debug("Reporting issue")
	} else {
		op = ops.Begin("send_email").Set("template", data.Template)
	}
	defer op.End()
	if data.Vars == nil {
		// avoid panicking in case the message is malformed
		data.Vars = make(map[string]interface{})
	}
	data.Vars["userID"] = st.GetUserID()
	data.Vars["proToken"] = st.GetToken()
	for k, v := range defaultVars {
		data.Vars[k] = v
	}
	if err := sendTemplate(data); err != nil {
		log.Error(op.FailIf(err))
		service.Out <- err.Error()
	} else {
		service.Out <- "success"
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
		if _, err := st.WriteTo(&buf); err != nil {
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
