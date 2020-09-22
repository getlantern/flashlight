package auth

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/getlantern/auth-server/models"
	"github.com/getlantern/auth-server/srp"
	"github.com/getlantern/lantern-server/common"
)

// The SRP client processes the server credentials
// and generates a mutual auth that is sent to the
// server as proof the client derived its keys
func (h AuthHandler) sendMutualAuth(srpClient *srp.SRPClient,
	w http.ResponseWriter,
	credentials, username string,
	onResponse common.HandleResponseFunc,
) error {
	cauth, err := srpClient.Generate(credentials)
	if err != nil {
		return err
	}
	params := &models.UserParams{
		MutualAuth: cauth,
		Username:   username,
	}
	requestBody, err := json.Marshal(params)
	if err != nil {
		return err
	}
	log.Debug("Sending mutual auth")
	url := h.GetAuthAddr(authEndpoint)
	req, err := http.NewRequest(common.POST, url, bytes.NewBuffer(requestBody))
	if err != nil {
		return err
	}
	req.Header.Set(common.HeaderContentType, common.MIMEApplicationJSON)
	h.ProxyHandler(url, req, w, onResponse)
	return nil
}

func (h AuthHandler) SendAuthRequest(method string, endpoint string, params *models.UserParams) (*http.Response, *models.AuthResponse, error) {
	requestBody, err := json.Marshal(params)
	if err != nil {
		return nil, nil, err
	}
	url := h.GetAuthAddr(endpoint)
	log.Debugf("Sending new auth request to %s", url)
	resp, err := h.DoHTTPRequest(method, url, requestBody)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	authResp, err := decodeAuthResponse(body)
	return resp, authResp, err
}

func (h AuthHandler) NewSRPClient(params models.UserParams, keepPassword bool) (*models.UserParams, *srp.SRPClient, error) {
	client, err := srp.NewSRPClient([]byte(params.Username),
		[]byte(params.Password))
	if err != nil {
		return nil, nil, err
	}
	ih, vh := client.Encode()
	// Remove user password since we have
	// a verifier now
	if !keepPassword {
		params.Password = ""
	}

	params.SRPIdentity = ih
	params.Verifier = vh
	params.Credentials = client.Credentials()
	return &params, client, nil
}

// GetSRPClient binds the provided request body to the userParams type
// and then creates a new SRP client instance from it
// The SRP parameters are attached to the returned user params
func (h AuthHandler) GetSRPClient(req *http.Request, keepPassword bool) (*models.UserParams, *srp.SRPClient, error) {
	var params models.UserParams
	err := common.DecodeJSONRequest(req, &params)
	if err != nil {
		return nil, nil, err
	}
	return h.NewSRPClient(params, keepPassword)
}
