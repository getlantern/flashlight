package app

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/getlantern/flashlight/ui"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestSendFromWS(t *testing.T) {
	addr := ui.GetDirectUIAddr()
	// ugly hack to co-exist with integration test: only start services if not
	// already started.
	if addr == "" {
		// avoid panicking when attaching settings to the email.
		settings = loadSettings("version", "revisionDate", "buildDate")
		err := serveMandrill()
		assert.NoError(t, err, "should start UI service")
		ui.Start("localhost:", false, "", "")
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
		if bytes.Contains(p, []byte("mandrill")) {
			assert.Equal(t, `{"type":"mandrill","message":"success"}`, string(p))
			break
		}
	}

	mandrillAPIKey = "SANDBOX_ERROR"
	err = sendTemplateVia(conn)
	assert.NoError(t, err, "should write to ws")
	for {
		_, p, err := conn.ReadMessage()
		assert.NoError(t, err, "should read from ws")
		if bytes.Contains(p, []byte("mandrill")) {
			assert.Equal(t, `{"type":"mandrill","message":"SANDBOX_ERROR"}`, string(p))
			break
		}
	}
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
