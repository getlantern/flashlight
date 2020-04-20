package auth

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/getlantern/flashlight/ui/handlers"
	"github.com/getlantern/golog"
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
	log                   = golog.LoggerFor("flashlight.ui.auth")
)

func withUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userKey, userID)
}

type AuthHandler struct {
	handlers.Handler
	proxy *httputil.ReverseProxy
}

func New(params handlers.Params) AuthHandler {
	u, err := url.Parse(params.AuthServerAddr)
	if err != nil {
		log.Fatal(fmt.Errorf("Bad auth server address: %s", params.AuthServerAddr))
	}
	return AuthHandler{
		handlers.New(params),
		httputil.NewSingleHostReverseProxy(u),
	}
}

func (h AuthHandler) Routes() map[string]handlers.HandlerFunc {
	proxyHandler := func(w http.ResponseWriter, r *http.Request) {
		h.proxy.ServeHTTP(w, r)
	}
	return map[string]handlers.HandlerFunc{
		"/login":       h.authHandler,
		"/register":    h.authHandler,
		"/user/logout": proxyHandler,
	}
}

// doRequest creates and sends a new HTTP request to the given url
// with an optional requestBody. It returns an HTTP response
func (h AuthHandler) doRequest(method, url string,
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
	return h.HttpClient.Do(req)
}

func (h AuthHandler) sendAuthRequest(method, url string,
	requestBody []byte) (*models.AuthResponse, error) {
	resp, err := h.doRequest(method, url, requestBody)
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
func (h AuthHandler) authHandler(w http.ResponseWriter, req *http.Request) {
	params, srpClient, err := h.getSRPClient(req)
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
		statusCode := http.StatusBadRequest
		if resp != nil {
			statusCode = resp.StatusCode
		}
		h.ErrorHandler(w, err, statusCode)
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
		authResp, err = h.sendMutualAuth(srpClient,
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
	h.ProxyHandler(req, w, onResp, onError)
}
