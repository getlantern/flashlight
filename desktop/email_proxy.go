package desktop

import (
	"bytes"
	"fmt"

	"github.com/getlantern/flashlight/diagnostics"
	"github.com/getlantern/flashlight/email"
	"github.com/getlantern/flashlight/ws"
	"github.com/getlantern/osversion"
	"github.com/getlantern/yaml"
)

type mandrillMessage struct {
	email.Message

	// If attach the settings file to the email or not
	WithSettings bool `json:"withSettings,omitempty"`

	// If true, diagnostics will be run and a report will be attached to the email. The email will
	// not be sent until the diagnostics have completed.
	RunDiagnostics bool `json:"runDiagnostics,omitempty"`
}

// A proxy that accept requests from WebSocket and send email via 3rd party
// service (mandrill atm). With optionally attached settings and Lantern logs.
// It intentionally uses direct connection to the 3rd party service, to serve
// as an out-of-band channel when Lantern doesn't work well, say, when user
// wants to report an issue.
func serveEmailProxy(channel ws.UIChannel) error {
	service, err := channel.RegisterWithMsgInitializer("email-proxy", nil,
		func() interface{} { return &mandrillMessage{} })
	if err != nil {
		log.Errorf("Error registering with UI? %v", err)
		return err
	}
	go read(service)
	return nil
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
	fillDefaults(data)
	if data.RunDiagnostics {
		data.DiagnosticsYAML = runDiagnostics()
	}
	if err := email.Send(&data.Message); err != nil {
		service.Out <- err.Error()
	} else {
		service.Out <- "success"
	}
}

func fillDefaults(msg *mandrillMessage) {
	if msg.Vars == nil {
		// avoid panicking in case the message is malformed
		msg.Vars = make(map[string]interface{})
	}
	msg.Vars["userID"] = settings.GetUserID()
	msg.Vars["deviceID"] = settings.GetDeviceID()
	msg.Vars["proToken"] = settings.GetToken()
	msg.Vars["version"] = fmt.Sprintf("%v (%v)",
		settings.getString(SNVersion),
		settings.getString(SNRevisionDate))
	os, err := osversion.GetHumanReadable()
	if err != nil {
		log.Errorf("Unable to get version: %v", err)
	} else {
		msg.Vars["os"] = os
	}
	if msg.WithSettings {
		buf := &bytes.Buffer{}
		_, err := settings.writeTo(buf)
		if err != nil {
			log.Errorf("Unable to serialize settings: %v", err)
			return
		}
		msg.SettingsData = buf.Bytes()
	}
}

// Returns nil and logs an error if encoding fails.
func runDiagnostics() (reportYAML []byte) {
	r := diagnostics.Run()
	b, err := yaml.Marshal(r)
	if err != nil {
		log.Errorf("failed to encode diagnostics report: %v", err)
		log.Debugf("the following report failed to encode: %+v", r)
		return nil
	}
	return b
}
