package android

import (
	"github.com/getlantern/flashlight/email"
)

// EmailMessage exposes the type email.Message as part of this package.
type EmailMessage email.Message

// EmailResponse is used to report any errors sending an email
type EmailResponse interface {
	ShowError(string)
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
	if msg.Vars == nil {
		msg.Vars = make(map[string]interface{})
	}
	msg.Vars[key] = val
}

// Send sends this EmailMessage using the email package.
func (msg *EmailMessage) Send(response EmailResponse) {
	emsg := email.Message(*msg)
	err := email.Send(&emsg)
	if err != nil {
		response.ShowError(err.Error())
	}
}
