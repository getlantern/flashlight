package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/getlantern/auth-server/api"
	"github.com/getlantern/auth-server/client"
	"github.com/getlantern/auth-server/models"
	"github.com/getlantern/flashlight/ui/handler"
	"github.com/getlantern/golog"
	"github.com/go-chi/chi"
)

var (
	ErrInvalidCredentials = errors.New("The supplied user credentials were invalid")
	ErrBadRequest         = errors.New("The request parameters were invalid")
	ErrSRPKeysDifferent   = errors.New("SRP client and server keys do not match")
	log                   = golog.LoggerFor("flashlight.ui.auth")
)

type AuthHandler struct {
	handler.Handler
	authClient *client.AuthClient
}

// New creates a new auth handler
func New(params api.APIParams) AuthHandler {
	return AuthHandler{
		handler.NewHandler(params),
		client.New(params.AuthServerAddr, params.HTTPClient),
	}
}

// GetPathPrefix returns the top-level route prefix used
// by the AuthHandler
func (h AuthHandler) GetPathPrefix() string {
	return "/user"
}

// getUserParams is used to unmarshal JSON from the given request r into
// the the user params type
func getUserParams(w http.ResponseWriter, r *http.Request) (*models.UserParams, error) {
	var params models.UserParams
	var err error
	switch r.Method {
	case http.MethodGet:
		// marshal query args into JSON
		b, err := json.Marshal(r.URL.Query())
		if err != nil {
			return nil, err
		}
		// now unmarshal target type user params
		err = json.Unmarshal(b, &params)
	default:
		err = handler.DecodeJSONRequest(w, r, &params)
	}
	// extract user credentials from HTTP request to send to AuthClient
	return &params, err
}

type AuthMethod func(params *models.UserParams) (api.AuthResponse, error)

// authHandler is the HTTP handler used by the login and registration endpoints.
// It creates a new SRP client from the user params in the request
func (h AuthHandler) authHandler(authenticate AuthMethod) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params, err := getUserParams(w, r)
		if err != nil {
			return
		}
		authResp, err := authenticate(params)
		handleResponse(authResp, w, err)
	})
}

// handleResponse returns the auth response to the client based on whether or not
// err is nil
func handleResponse(authResp api.AuthResponse, w http.ResponseWriter, err error) {
	if err != nil {
		handler.ErrorHandler(w, err, authResp.StatusCode)
	} else {
		handler.SuccessResponse(w, authResp)
	}
}

// ConfigureRoutes returns an http.Handler for the auth-based routes
func (h AuthHandler) ConfigureRoutes() http.Handler {
	r := handler.NewRouter()
	r.Group(func(r chi.Router) {
		r.Post("/login", h.authHandler(h.authClient.SignIn))
		r.Post("/register", h.authHandler(h.authClient.Register))
		r.Get("/account/status", func(w http.ResponseWriter, r *http.Request) {
			params, err := getUserParams(w, r)
			if err != nil {
				return
			}
			authResp, err := h.authClient.AccountStatus(params)
			handleResponse(authResp, w, err)
		})
		r.Post("/logout", func(w http.ResponseWriter, r *http.Request) {
			params, err := getUserParams(w, r)
			if err != nil {
				return
			}
			authResp, err := h.authClient.SignOut(params.Username)
			if err != nil {
				log.Debugf("User %s successfully signed out", params.Username)
			}
			handleResponse(authResp, w, err)
		})
	})
	return r
}
