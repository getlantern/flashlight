package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/getlantern/auth-server/models"
	"github.com/getlantern/auth-server/srp"
	"github.com/getlantern/flashlight/ui/api"
	"github.com/getlantern/flashlight/ui/handler"
	"github.com/getlantern/golog"
	"github.com/getlantern/lantern-server/common"
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
	handler.Handler
	proxy *httputil.ReverseProxy
}

func New(params api.Params) AuthHandler {
	u, err := url.Parse(params.AuthServerAddr)
	if err != nil {
		log.Fatal(fmt.Errorf("Bad auth server address: %s", params.AuthServerAddr))
	}
	return AuthHandler{
		handler.NewHandler(params),
		httputil.NewSingleHostReverseProxy(u),
	}
}

func (h AuthHandler) Routes() map[string]handler.HandlerFunc {
	proxyHandler := func(w http.ResponseWriter, r *http.Request) {
		h.proxy.ServeHTTP(w, r)
	}
	return map[string]handler.HandlerFunc{
		"/login":       h.authHandler,
		"/register":    h.authHandler,
		"/user/logout": proxyHandler,
	}
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
	w http.ResponseWriter,
	params *models.UserParams, authResp *models.AuthResponse) error {
	if authResp.Error != "" {
		return errors.New(authResp.Error)
	}
	onResp := func(resp *http.Response) error {
		body, err := common.ReadResponseBody(resp)
		if err != nil {
			return err
		}
		log.Debugf("Got mutual auth response: %v", string(body))
		authResp, err := decodeAuthResponse(body)
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

	// client generates a mutual auth and sends it to the server
	return h.sendMutualAuth(srpClient, w,
		authResp.Credentials, params.Username, onResp)
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
	endpoint := html.EscapeString(req.URL.Path)
	h.SendAuth(w, endpoint, srpClient, params)
}

func (h AuthHandler) SendAuth(w http.ResponseWriter, endpoint string,
	srpClient *srp.SRPClient,
	params *models.UserParams) {
	resp, authResp, err := h.SendAuthRequest(common.POST, endpoint, params)
	if err != nil || resp.StatusCode != http.StatusOK {
		if err != nil {
			log.Error(err)
		}
		if authResp != nil {
			if authResp.Error != "" {
				h.ErrorHandler(w, errors.New(authResp.Error), http.StatusBadRequest)
			}
			h.ErrorHandler(w, authResp.Errors, http.StatusBadRequest)
		}
		return
	}
	h.HandleAuthResponse(srpClient, w, params, authResp)
}
