package android

import (
	"github.com/getlantern/flashlight/email"
)

// EmailMessage exposes the type email.Message as part of this package.
type EmailMessage struct {
	*email.Message
}

// SendEmail sends the given EmailMessage using the email package.
func SendEmail(msg EmailMessage) error {
	return email.Send(msg.Message)
}
