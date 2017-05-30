package android

import (
	"github.com/getlantern/flashlight/email"
)

// EmailMessage exposes the type email.Message as part of this package.
type EmailMessage struct {
	// The mandrill template slug
	Template string
	// The email address to which the message is sent
	To string
	// Specify the maximum size of the not-compressed log files attached to the
	// email, can be in "KB/MB/GB" fromat. Not attaching logs if it's an empty
	// string.  Make sure the compressed files doesn't cause the message to exceed
	// the limit of Mandrill message size, which is 25MB.
	MaxLogSize string `json:"maxLogSize,omitempty"`
	// Any global vars defined in the template
	vars map[string]interface{}
}

// PutInt sets an integer variable
func (msg *EmailMessage) PutInt(key string, val int) {
	msg.putVar(key, val)
}

// PutString sets a string variable
func (msg *EmailMessage) PutString(key string, val string) {
	msg.putVar(key, val)
}

func (msg *EmailMessage) putVar(key string, val interface{}) {
	if msg.vars == nil {
		msg.vars = make(map[string]interface{})
	}
	msg.vars[key] = val
}

// Send sends this EmailMessage using the email package.
func (msg *EmailMessage) Send() {
	email.Send(&email.Message{
		Template:   msg.Template,
		To:         msg.To,
		Vars:       msg.vars,
		MaxLogSize: msg.MaxLogSize,
	})
}
