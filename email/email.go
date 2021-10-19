// Package email provides functionality for sending email messages via Mandrill
package email

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	pops "github.com/getlantern/ops"
	"github.com/getlantern/yaml"
	"github.com/keighl/mandrill"

	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/util"
)

var (
	log = golog.LoggerFor("flashlight.email")

	// Number of runes(code points - characters of variable length bytes depending on encoding) allowed in fileName length
	maxNameLength uint = 60
	// Number of bytes allowed in file attachment (8 mb)
	maxFileSize      float64 = 8 * math.Pow(10, 6)
	defaultRecipient string
	httpClient       = &http.Client{}
	mu               sync.RWMutex
)

// Only allowed to call /send_template. Just make it more annoying
// to determine the key by examining the binary by encoding it in hex.
// Just thwart casual attackers for a relatively low value key.
var Key = "5279526e53385644704f6f703147706f56796e435a41"

func init() {
	b, _ := hex.DecodeString(Key)
	Key = string(b)
}

// SetDefaultRecipient configures the email address that will receive emails
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

// SetHTTPClient configures an alternate http.Client to use when sending emails
func SetHTTPClient(client *http.Client) {
	mu.Lock()
	defer mu.Unlock()
	httpClient = client
}

func getHTTPClient() *http.Client {
	mu.RLock()
	defer mu.RUnlock()
	return httpClient
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
	// Logs allows the caller to specify an already zipped set of logs
	Logs []byte
	// DiagnosticsYAML is a YAML-encoded diagnostics report.
	DiagnosticsYAML []byte
	// ProxyCapture is a gzipped pcapng file related to diagnostics.
	ProxyCapture []byte
	// Proxies allows the caller to specify a proxies.yaml file
	Proxies []byte
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
				Set("issue_note", msg.Vars["note"])
		} else {
			// get parameters from android template vars
			isPro, _ := strconv.ParseBool(fmt.Sprint(msg.Vars["prouser"]))
			op.Set("pro", isPro).
				Set("issue_type", msg.Vars["issue"]).
				Set("issue_note", msg.Vars["report"])
		}
		msg.Vars["country"] = geolookup.GetCountry(0)
		log.Debug("Reporting issue")
	} else {
		op = ops.Begin("send_email").Set("template", msg.Template)
	}
	defer op.End()
	err := sendTemplate(msg)
	if err != nil {
		return log.Error(op.FailIf(err))
	}
	return nil
}

func sendTemplate(msg *Message) error {
	client := mandrill.ClientWithKey(Key)
	client.HTTPClient = getHTTPClient()
	recipient := msg.To
	if recipient == "" {
		recipient = getDefaultRecipient()
	}
	if recipient == "" {
		return errors.New("no recipient")
	}
	mmsg := &mandrill.Message{
		FromEmail: msg.From,
		To:        []*mandrill.To{{Email: recipient}},
		Subject:   msg.Subject,
	}
	if msg.CC != "" {
		mmsg.To = append(mmsg.To, &mandrill.To{Email: msg.CC, Type: "cc"})
	}
	if msg.Vars["file"] != nil {
		fileName := fmt.Sprintf("%v", msg.Vars["fileName"])
		fileName = util.TrimStringAsRunes(maxNameLength, fileName, true)
		fileName = util.SanitizePathString(fileName)

		fileContent := fmt.Sprintf("%v", msg.Vars["file"])
		byteLen := float64(len(fileContent))
		if byteLen <= maxFileSize {
			mmsg.Attachments = append(mmsg.Attachments, &mandrill.Attachment{
				Type:    fmt.Sprintf("%v", msg.Vars["fileType"]),
				Name:    fileName,
				Content: fileContent,
			})
		} else {
			return errors.New("file too large")
		}
	}
	mmsg.GlobalMergeVars = mandrill.MapToVars(msg.Vars)
	if msg.SettingsData != nil {
		mmsg.Attachments = append(mmsg.Attachments, &mandrill.Attachment{
			Type:    "application/x-yaml",
			Name:    prefix(msg) + "_settings.yaml",
			Content: base64.StdEncoding.EncodeToString(msg.SettingsData),
		})
		attachOpsCtx(msg, mmsg)
	}
	if msg.DiagnosticsYAML != nil {
		mmsg.Attachments = append(mmsg.Attachments, &mandrill.Attachment{
			Type:    "application/x-yaml",
			Name:    prefix(msg) + "_diagnostics.yaml",
			Content: base64.StdEncoding.EncodeToString(msg.DiagnosticsYAML),
		})
	}
	if msg.ProxyCapture != nil {
		mmsg.Attachments = append(mmsg.Attachments, &mandrill.Attachment{
			Type:    "application/zip",
			Name:    prefix(msg) + "_proxy_capture.zip",
			Content: base64.StdEncoding.EncodeToString(msg.ProxyCapture),
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
	if len(msg.Logs) > 0 {
		mmsg.Attachments = append(mmsg.Attachments, &mandrill.Attachment{
			Type:    "application/zip",
			Name:    "logs.zip",
			Content: base64.StdEncoding.EncodeToString(msg.Logs),
		})
	}
	if len(msg.Proxies) > 0 {
		mmsg.Attachments = append(mmsg.Attachments, &mandrill.Attachment{
			Type:    "text/yaml",
			Name:    "proxies.yaml",
			Content: base64.StdEncoding.EncodeToString(msg.Proxies),
		})
	} else {
		log.Debug("No proxies.yaml included to send to mandrill")
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

func attachOpsCtx(msg *Message, mmsg *mandrill.Message) {
	defer func() {
		p := recover()
		if p != nil {
			log.Errorf("Panicked while trying to attach ops context to mandrill message, continuing with submission: %v", p)
		}
	}()

	opsCtx := pops.AsMap(nil, true)
	opsCtxYAML, err := yaml.Marshal(opsCtx)
	if err != nil {
		log.Errorf("Unable to marshal global ops context to JSON: %v", err)
	} else {
		mmsg.Attachments = append(mmsg.Attachments, &mandrill.Attachment{
			Type:    "application/x-yaml",
			Name:    prefix(msg) + "_context.yaml",
			Content: base64.StdEncoding.EncodeToString(opsCtxYAML),
		})
	}
}
