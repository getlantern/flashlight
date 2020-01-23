package ui

import (
	"encoding/json"
	"net/http"

	"github.com/getlantern/lantern-server/models"
	"github.com/getlantern/lantern-server/srp"
)

func (s *Server) sendMutualAuth(srpClient *srp.SRPClient,
	credentials, username string) (*models.AuthResponse, error) {
	cauth, err := srpClient.Generate(credentials)
	if err != nil {
		return nil, err
	}
	params := &models.AuthParams{
		MutualAuth: cauth,
		Username:   username,
	}
	requestBody, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	url := s.getAPIAddr(authEndpoint)
	log.Debugf("Sending mutual auth to %s", url)
	return s.sendAuthRequest(POST, url, requestBody)
}

// getSRPClient binds the provided request body to the userParams type
// and then creates a new SRP client instance from it
// The SRP parameters are attached to the returned user params
func (s *Server) getSRPClient(req *http.Request) (*models.UserParams, *srp.SRPClient, error) {
	var params models.UserParams
	err := decodeJSONRequest(req, &params)
	if err != nil {
		return nil, nil, err
	}
	username := params.Username
	client, err := srp.NewSRPClient([]byte(username),
		[]byte(params.Password))
	if err != nil {
		return nil, nil, err
	}
	ih, vh := client.Encode()

	// Remove user password since we have
	// a verifier now
	params.Password = ""

	params.SRPIdentity = ih
	params.Verifier = vh
	params.Credentials = client.Credentials()
	return &params, client, nil
}
