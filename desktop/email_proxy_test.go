package desktop

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/email"
	"github.com/getlantern/flashlight/ws"
)

func TestEmailProxy(t *testing.T) {
	channel := ws.NewUIChannel()
	s := httptest.NewServer(channel.Handler())
	defer s.Close()
	// avoid panicking when attaching settings to the email.
	setSettings(loadSettingsFrom("version", "revisionDate", "buildDate", "", newChromeExtension()))
	err := (&App{}).serveEmailProxy(channel)
	assert.NoError(t, err, "should start service")
	defer channel.Unregister("email-proxy")
	wsURL := strings.Replace(s.URL, "http://", "ws://", -1)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{})
	if !assert.NoError(t, err, "should connect to Websocket") {
		return
	}

	defer func() { _ = conn.Close() }()
	email.MandrillAPIKey = "SANDBOX_SUCCESS"
	err = sendTemplateVia(conn)
	assert.NoError(t, err, "should write to ws")
	_, p, err := conn.ReadMessage()
	assert.NoError(t, err, "should read from ws")
	assert.Equal(t, `{"type":"email-proxy","message":"success"}`, string(p))

	email.MandrillAPIKey = "SANDBOX_ERROR"
	err = sendTemplateVia(conn)
	assert.NoError(t, err, "should write to ws")
	_, p, err = conn.ReadMessage()
	assert.NoError(t, err, "should read from ws")
	assert.Equal(t, `{"type":"email-proxy","message":"SANDBOX_ERROR"}`, string(p))
}

func sendTemplateVia(conn *websocket.Conn) error {
	return conn.WriteMessage(websocket.TextMessage, []byte(`{
		"type":"email-proxy",
		"message": {
			"template":     "user-send-logs-desktop",
			"to":           "fffw@getlantern.org",
			"withSettings": true,
			"maxLogSize":   "5MB",
			"vars": {
				"userID": "1234",
				"email":  "user@lantern.org",
				"OS":     "Windows"
			}
		}
	}`))
}
