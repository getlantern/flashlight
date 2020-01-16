package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/lantern-server/models"
	"github.com/getlantern/lantern-server/srp"
	"github.com/stretchr/testify/assert"
)

func getClient(s *Server, t *testing.T, username, password string) *srp.SRPClient {
	params := &models.UserParams{
		Username: username,
		Password: password,
	}
	requestBody, _ := json.Marshal(params)
	req, _ := http.NewRequest("GET", "/login", bytes.NewBuffer(requestBody))
	req.Header.Set(HeaderContentType, MIMEApplicationJSON)
	_, client, err := s.getSRPClient(req)
	assert.NoError(t, err, "Should be no error creating SRP client")
	assert.NotNil(t, client)
	return client
}

func startServer(t *testing.T, authaddr, addr string) *Server {
	s := newServer("", authaddr, "test-http-token", false)
	assert.NoError(t, s.start(addr), "should start server")
	return s
}

func TestGetSRPClient(t *testing.T) {
	s := startServer(t, common.AuthServerAddr, "")
	username := "user1234"
	password := "p@sswor1234"
	getClient(s, t, username, password)
}

func TestSRP(t *testing.T) {

}
