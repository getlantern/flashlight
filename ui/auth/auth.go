package ui

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"

	"github.com/getlantern/lantern-server/common"
	"github.com/getlantern/lantern-server/models"
	"github.com/getlantern/lantern-server/srp"
)

const (
	userKey               = iota
	authEndpoint          = "/auth"
	loginEndpoint         = "/login"
	registrationEndpoint  = "/register"
	balanceEndpoint       = "/user/balance"
	createAccountEndpoint = "/user/account/new"
)

var (
	ErrInvalidCredentials = errors.New("The supplied user credentials were invalid")
	ErrSRPKeysDifferent   = errors.New("SRP client and server keys do not match")
)

func withUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userKey, userID)
}

// getAPIAddr combines the given uri with the authentication server address
func (s *Server) getAPIAddr(uri string) string {
	return fmt.Sprintf("%s%s", s.authServerAddr, uri)
}

// proxyHandler is a HTTP handler used to proxy requests
// to the Lantern authentication server
func (s *Server) proxyHandler(req *http.Request, w http.ResponseWriter,
	onResponse common.HandleResponseFunc,
	onError common.HandleErrorFunc,
) {
	url := s.getAPIAddr(html.EscapeString(req.URL.Path))
	common.ProxyHandler(url, s.httpClient, req, w,
		onResponse,
		onError)
}

// doRequest creates and sends a new HTTP request to the given url
// with an optional requestBody. It returns an HTTP response
func (s *Server) doRequest(method, url string,
	requestBody []byte) (*http.Response, error) {
	log.Debugf("Sending new request to url %s", url)
	var req *http.Request
	var err error
	if requestBody != nil {
		req, err = http.NewRequest(method, url,
			bytes.NewBuffer(requestBody))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set(common.HeaderContentType, common.MIMEApplicationJSON)
	return s.httpClient.Do(req)
}

func (s *Server) sendAuthRequest(method, url string,
	requestBody []byte) (*models.AuthResponse, error) {
	resp, err := s.doRequest(method, url, requestBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return decodeAuthResponse(body)
}

func decodeAuthResponse(body []byte) (*models.AuthResponse, error) {
	authResp := new(models.AuthResponse)
	err := json.Unmarshal(body, authResp)
	return authResp, err
}

// authHandler is the HTTP handler used by the login and
// registration endpoints. It creates a new SRP client from
// the user params in the request and sends the
// SRP params (i.e. verifier) generated to the authentication
// server, establishing a fully authenticated session
func (s *Server) authHandler(w http.ResponseWriter, req *http.Request) {
	params, srpClient, err := s.getSRPClient(req)
	if err != nil {
		return
	}
	requestBody, err := json.Marshal(params)
	if err != nil {
		return
	}
	req.Body = ioutil.NopCloser(bytes.NewBuffer(requestBody))

	onError := func(resp *http.Response, err error) {
		log.Debugf("Encountered error processing auth response: %v", err)
		s.errorHandler(w, err, resp.StatusCode)
	}

	onResp := func(resp *http.Response, body []byte) error {
		authResp, err := decodeAuthResponse(body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK || authResp.Error != "" {
			err = errors.New(authResp.Error)
			return err
		}
		// client generates a mutual auth and sends it to the server
		authResp, err = s.sendMutualAuth(srpClient,
			authResp.Credentials, params.Username)
		if err != nil {
			return err
		}
		// Verify the server's proof
		ok := srpClient.ServerOk(authResp.Proof)
		if !ok {
			return ErrInvalidCredentials
		}
		srv, err := srp.UnmarshalServer(authResp.Server)
		if err != nil {
			return err
		}
		// Client and server are successfully authenticated to each other
		kc := srpClient.RawKey()
		ks := srv.RawKey()
		if 1 != subtle.ConstantTimeCompare(kc, ks) {
			return ErrSRPKeysDifferent
		}
		log.Debug("Successfully created new SRP session")
		return nil
	}
	s.proxyHandler(req, w, onResp, onError)
}
