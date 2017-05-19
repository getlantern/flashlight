package emailproxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/keighl/mandrill"
	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/desktop/settings"
	"github.com/getlantern/flashlight/ws"
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
	s := httptest.NewServer(ws.StartUIChannel())
	defer s.Close()
	// settings = ("version", "revisionDate", "buildDate")
	err := Start(settings.New(), "devID", "version", "revisionDate")
	assert.NoError(t, err, "should start service")
	defer ws.Unregister("email-proxy")
	wsURL := strings.Replace(s.URL, "http://", "ws://", -1)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{})
	if !assert.NoError(t, err, "should connect to Websocket") {
		return
	}

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
