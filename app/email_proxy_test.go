package app

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/getlantern/flashlight/ui"
	"github.com/gorilla/websocket"
	"github.com/keighl/mandrill"
	"github.com/stretchr/testify/assert"
)

func TestReadResponses(t *testing.T) {

	// Here are the various response statuses from
	// https://github.com/keighl/mandrill/blob/master/mandrill.go#L186
	// the sending status of the recipient - either "sent", "queued", "scheduled", "rejected", or "invalid"

	statuses := []string{
		"sent", "queued", "scheduled", "rejected", "invalid",
	}

	for _, status := range statuses {
		var responses []*mandrill.Response
		responses = append(responses, &mandrill.Response{Status: status})
		err := readResponses(responses)
		if status == "sent" || status == "queued" || status == "scheduled" {
			assert.Nil(t, err, "Expected no error for status "+status)
		} else if status == "rejected" || status == "invalid" {
			assert.False(t, err == nil)
		}
	}

}

func TestEmailProxy(t *testing.T) {
	// ugly hack to co-exist with integration test: only start services if not
	// already started.
	addr := ui.GetDirectUIAddr()
	if addr == "" {
		// avoid panicking when attaching settings to the email.
		settings = loadSettings("version", "revisionDate", "buildDate")
		err := serveEmailProxy()
		assert.NoError(t, err, "should start UI service")
		ui.Start("localhost:", false, "", "")
		defer func() { ui.Stop() }()
		addr = ui.GetDirectUIAddr()
	}
	wsURL := "ws://" + addr + "/data"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{})
	assert.NoError(t, err, "should connect to Websocket")
	defer func() { _ = conn.Close() }()

	mandrillAPIKey = "SANDBOX_SUCCESS"
	err = sendTemplateVia(conn)
	assert.NoError(t, err, "should write to ws")
	// When running with integration test, there're some other WebSocket
	// messages sent to client. Filter mandrill specific message to verify. If
	// there's no such message, the test hangs as an indication. Same below.
	for {
		_, p, err := conn.ReadMessage()
		assert.NoError(t, err, "should read from ws")
		if bytes.Contains(p, []byte("email-proxy")) {
			assert.Equal(t, `{"type":"email-proxy","message":"success"}`, string(p))
			break
		}
	}

	mandrillAPIKey = "SANDBOX_ERROR"
	err = sendTemplateVia(conn)
	assert.NoError(t, err, "should write to ws")
	for {
		_, p, err := conn.ReadMessage()
		assert.NoError(t, err, "should read from ws")
		if bytes.Contains(p, []byte("email-proxy")) {
			assert.Equal(t, `{"type":"email-proxy","message":"SANDBOX_ERROR"}`, string(p))
			break
		}
	}
}

func sendTemplateVia(conn *websocket.Conn) error {
	return conn.WriteJSON(ui.Envelope{
		EnvelopeType: ui.EnvelopeType{
			Type: "email-proxy",
		},
		Message: mandrillMessage{
			Template:     "user-send-logs-desktop",
			To:           "fffw@getlantern.org",
			WithSettings: true,
			MaxLogSize:   "5MB",
			Vars: map[string]interface{}{
				"userID": "1234",
				"email":  "user@lantern.org",
				"OS":     "Windows",
			},
		},
	})
}
