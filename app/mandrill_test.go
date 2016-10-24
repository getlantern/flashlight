package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/getlantern/flashlight/ui"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestSendFromWS(t *testing.T) {
	// intialize package level var, to avoid panicking when attaching settings
	// to the email.
	settings = loadSettings("version", "revisionDate", "buildDate")
	err := serveMandrill()
	assert.NoError(t, err, "should start UI service")
	ui.Start("localhost:", false, "")
	wsURL := strings.Replace(ui.UIAddr(), "http", "ws", 1) + "/data"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{})
	assert.NoError(t, err, "should connect to Websocket")
	defer func() { _ = conn.Close() }()
	// ignore the first message sent to client: the settings
	_, p, err := conn.ReadMessage()

	mandrillAPIKey = "SANDBOX_SUCCESS"
	err = sendTemplateVia(conn)
	assert.NoError(t, err, "should write to ws")
	_, p, err = conn.ReadMessage()
	assert.NoError(t, err, "should read from ws")
	assert.Equal(t, `{"type":"mandrill","message":"success"}`, string(p))

	mandrillAPIKey = "SANDBOX_ERROR"
	err = sendTemplateVia(conn)
	assert.NoError(t, err, "should write to ws")
	_, p, err = conn.ReadMessage()
	assert.NoError(t, err, "should read from ws")
	assert.Equal(t, `{"type":"mandrill","message":"SANDBOX_ERROR"}`, string(p))
}

func sendTemplateVia(conn *websocket.Conn) error {
	return conn.WriteJSON(ui.Envelope{
		EnvelopeType: ui.EnvelopeType{
			Type: "mandrill",
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
