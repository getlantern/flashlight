package client

import (
	"errors"
	"github.com/getlantern/flashlight/proxied"
)

type ProRequest struct {
	Client *Client
	User   User
}

func NewRequest(shouldProxy bool, user User) (*ProRequest, error) {
	httpClient, err := proxied.GetHTTPClient(shouldProxy)
	if err != nil {
		log.Errorf("Could not create HTTP client: %v", err)
		return nil, err
	}

	req := &ProRequest{
		Client: NewClient(httpClient),
		User:   user,
	}

	return req, nil
}

func UserStatus(req *ProRequest) (*Response, error) {

	res, err := req.Client.UserData(req.User)
	if err != nil {
		log.Errorf("Failed to get user data: %v", err)
		return nil, err
	}

	if res.Status == "error" {
		return nil, errors.New(res.Error)
	}
	return res, nil
}
