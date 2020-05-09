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
	ErrBadRequest         = errors.New("The request parameters were invalid")
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

func (h AuthHandler) sendAuthRequest(method, url string,
	requestBody []byte) (*models.AuthResponse, error) {
	log.Debugf("Sending new auth request to %s", url)
	resp, err := h.DoRequest(method, url, requestBody)
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

func (h AuthHandler) HandleAuthError(w http.ResponseWriter,
	resp *http.Response, err error) {
	log.Debugf("Encountered error processing auth response: %v", err)
	statusCode := http.StatusBadRequest
	if resp != nil {
		statusCode = resp.StatusCode
	}
	h.ErrorHandler(w, err, statusCode)
}

// HandleAuthResponse sends the SRP params (i.e. verifier) generated
// on the client to the authentication server, establishing
// a fully authenticated session
func (h AuthHandler) HandleAuthResponse(srpClient *srp.SRPClient,
	params *models.UserParams,
	resp *http.Response) error {
	if resp.StatusCode != http.StatusOK {
		return errors.New("Received an invalid response from auth server")
	}
	body, err := common.ReadResponseBody(resp)
	if err != nil {
		return err
	}
	authResp, err := decodeAuthResponse(body)
	if err != nil {
		return err
	}
	log.Debugf("Auth Response: status %v %v",
		resp.StatusCode, authResp)
	if authResp.Error != "" {
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

// authHandler is the HTTP handler used by the login and
// registration endpoints. It creates a new SRP client from
// the user params in the request
func (h AuthHandler) authHandler(w http.ResponseWriter, req *http.Request) {
	params, srpClient, err := h.GetSRPClient(req, false)
	if err != nil {
		log.Errorf("Couldn't create SRP client from request: %v", err)
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	requestBody, err := json.Marshal(params)
	if err != nil {
		log.Errorf("Error marshaling request body: %v", err)
		h.ErrorHandler(w, err, http.StatusBadRequest)
		return
	}
	req.Body = ioutil.NopCloser(bytes.NewBuffer(requestBody))
	h.ProxyHandler(req, w, func(resp *http.Response) error {
		return h.HandleAuthResponse(srpClient, params, resp)
	})
}
